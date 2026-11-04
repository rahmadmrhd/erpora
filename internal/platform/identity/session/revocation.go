package session

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"erpora/internal/platform/identity/store"
	"erpora/internal/platform/identity/token"
	"erpora/internal/platform/tenancy"
)

// DefaultReloadInterval adalah selang muat ulang berkala daftar tercabut dari DB (pengaman notifikasi hilang).
const DefaultReloadInterval = time.Minute

const pruneEvery = time.Minute

// RevokedLoader memuat sesi tercabut dari persistensi (dipenuhi *store.Store).
type RevokedLoader interface {
	ListRevokedSessionsSince(ctx context.Context, t tenancy.TenantID, since time.Time) ([]store.RevokedSession, error)
}

// RevocationConfig mengonfigurasi RevocationList.
type RevocationConfig struct {
	// Loader memuat daftar awal dan muat ulang (wajib bila Load/Subscribe dipakai).
	Loader RevokedLoader
	// Now adalah jam (bisa diinjeksi pada tes); nil berarti time.Now.
	Now func() time.Time
	// Log adalah logger (nil berarti slog.Default).
	Log *slog.Logger
}

// RevocationList adalah daftar sesi tercabut di memori (peta sid -> batas berlaku di belakang sync.RWMutex).
// Middleware Authenticate memanggil IsRevoked(sid) pada setiap permintaan tanpa query DB (ADR-0013 §2).
//
// Entri disimpan sampai RevokedAt + RevocationRetention (umur access token + leeway verifier = 10 menit 30
// detik): selama itu token terakhir yang mungkin beredar masih diterima verifier, sehingga entri tidak boleh dibuang
// lebih awal. Batas atas keberlakuan tiap entri adalah retensi itu sendiri; notifikasi yang hilang tidak
// membuat sesi tercabut valid selamanya (batasnya exp token + leeway), dan muat ulang berkala menutup celahnya.
type RevocationList struct {
	loader RevokedLoader
	now    func() time.Time
	log    *slog.Logger

	mu        sync.RWMutex
	until     map[string]time.Time
	lastPrune time.Time
}

// NewRevocationList membuat daftar kosong. Panggil Load saat startup sebelum melayani permintaan.
func NewRevocationList(cfg RevocationConfig) *RevocationList {
	l := &RevocationList{loader: cfg.Loader, now: cfg.Now, log: cfg.Log, until: map[string]time.Time{}}
	if l.now == nil {
		l.now = time.Now
	}
	if l.log == nil {
		l.log = slog.Default()
	}
	l.lastPrune = l.now()
	return l
}

// IsRevoked melaporkan apakah sesi sid masih tercatat tercabut (batas berlaku inklusif). Murah dan aman
// dipanggil serentak.
func (l *RevocationList) IsRevoked(sid string) bool {
	if sid == "" {
		return false
	}
	now := l.now()
	l.mu.RLock()
	until, ok := l.until[sid]
	l.mu.RUnlock()
	return ok && !now.After(until)
}

// Add mencatat sid tercabut sampai until (biasanya RevokedAt + RevocationRetention). Entri yang sudah ada tidak
// pernah dipersingkat. sid kosong atau until yang sudah lewat diabaikan.
func (l *RevocationList) Add(sid string, until time.Time) {
	if sid == "" {
		return
	}
	now := l.now()
	if now.After(until) {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if cur, ok := l.until[sid]; !ok || until.After(cur) {
		l.until[sid] = until
	}
	if now.Sub(l.lastPrune) >= pruneEvery {
		l.pruneLocked(now)
	}
}

// Len adalah jumlah entri yang tersimpan (termasuk yang sudah lewat tetapi belum dipangkas); untuk diagnostik.
func (l *RevocationList) Len() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.until)
}

// Prune membuang entri yang sudah melewati batas berlakunya (tidak pernah lebih awal dari batas itu).
func (l *RevocationList) Prune() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pruneLocked(l.now())
}

func (l *RevocationList) pruneLocked(now time.Time) {
	for sid, until := range l.until {
		if now.After(until) {
			delete(l.until, sid)
		}
	}
	l.lastPrune = now
}

// Load memuat sesi tenant (dari context) yang dicabut dalam jendela retensi dan menambahkannya ke daftar
// (digabung, tidak menimpa). Dipanggil saat startup (gagal tertutup: bila galat, instance tidak boleh melayani
// permintaan karena himpunan sesi tercabutnya tidak diketahui) dan oleh muat ulang Subscribe. Pada mode
// multi-tenant, panggil per tenant.
func (l *RevocationList) Load(ctx context.Context) error {
	if l.loader == nil {
		return errors.New("session: RevocationList tanpa Loader")
	}
	t, ok := tenancy.FromContext(ctx)
	if !ok {
		return ErrNoTenant
	}
	now := l.now()
	rows, err := l.loader.ListRevokedSessionsSince(ctx, t, now.Add(-RevocationRetention))
	if err != nil {
		return err
	}
	for _, r := range rows {
		l.Add(r.ID, r.RevokedAt.Add(RevocationRetention))
	}
	return nil
}

// Subscribe menyambungkan daftar ke notifier kanal ChannelSessionRevoked dan memulai satu goroutine pekerja
// yang memuat ulang dari DB (lewat Load, memakai ctx yang membawa tenant) setiap ada notifikasi dan setiap
// reloadEvery (0 = DefaultReloadInterval; negatif = tanpa muat ulang berkala). token.Notifier tidak membawa
// muatan, jadi notifikasi hanya sinyal "ada pencabutan" dan sumber kebenarannya DB; sinyal beruntun digabung
// (satu muat ulang tertunda), sehingga banjir pencabutan tidak membanjiri DB. Satu muat ulang dijalankan segera
// setelah berlangganan agar celah antara Load awal dan langganan tertutup. Hasilnya adalah fungsi penutup
// idempoten yang berhenti berlangganan dan menunggu goroutine berhenti (tidak ada goroutine bocor). ctx yang
// dibatalkan juga menghentikan pekerja.
func (l *RevocationList) Subscribe(ctx context.Context, n token.Notifier, reloadEvery time.Duration) (cancel func()) {
	if reloadEvery == 0 {
		reloadEvery = DefaultReloadInterval
	}
	wctx, stop := context.WithCancel(ctx)
	kick := make(chan struct{}, 1)
	kick <- struct{}{} // muat ulang awal
	signal := func() {
		select {
		case kick <- struct{}{}:
		default: // sudah ada muat ulang tertunda
		}
	}
	var unsub func()
	if n != nil {
		unsub = n.Subscribe(ChannelSessionRevoked, func(context.Context) { signal() })
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		var tick <-chan time.Time
		if reloadEvery > 0 {
			tk := time.NewTicker(reloadEvery)
			defer tk.Stop()
			tick = tk.C
		}
		for {
			select {
			case <-wctx.Done():
				return
			case <-kick:
			case <-tick:
			}
			if err := l.Load(wctx); err != nil && wctx.Err() == nil {
				l.log.WarnContext(wctx, "muat ulang daftar sesi tercabut gagal", "error", err.Error())
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			if unsub != nil {
				unsub()
			}
			stop()
			<-done
		})
	}
}
