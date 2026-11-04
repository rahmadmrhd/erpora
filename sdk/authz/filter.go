package authz

import (
	"context"
	"log/slog"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Filter mengembalikan GORM scope untuk query daftar: menambah klausa WHERE berparameter yang membatasi
// baris pada yang boleh dilakukan principal untuk aksi pada resource (FR-111, FR-115). Antarbaris matriks
// digabung OR dalam satu kelompok yang di-AND dengan kondisi pemanggil; kondisi pemanggil tidak pernah
// ikut di-OR. Gagal tertutup (NFR-106): tanpa principal/hak sistem, tanpa mesin, galat atau panic mesin,
// dan scope yang rusak menghasilkan WHERE false, tidak pernah "semua baris".
func Filter(ctx context.Context, resource, action string) func(*gorm.DB) *gorm.DB {
	rf, err := engineFilter(ctx, resource, action)
	return func(db *gorm.DB) *gorm.DB {
		if db == nil {
			return nil
		}
		if err != nil {
			return db.Where(falseExpr)
		}
		return applyRowFilter(ctx, db, rf)
	}
}

var falseExpr = clause.Expr{SQL: "false"}

func engineFilter(ctx context.Context, resource, action string) (rf RowFilter, err error) {
	e, ok := engineFrom(ctx)
	if !ok || !hasActor(ctx) {
		return RowFilter{}, ErrForbidden
	}
	defer func() {
		if r := recover(); r != nil {
			slog.ErrorContext(ctx, "authz: panic pada Engine.Filter", "resource", resource, "action", action, "panic", r)
			rf, err = RowFilter{}, ErrForbidden
		}
	}()
	return e.Filter(ctx, resource, action)
}

// applyRowFilter menerjemahkan RowFilter ke klausa GORM. Cabang yang bernilai FALSE dilepas dari OR;
// bila tidak ada cabang yang tersisa hasilnya WHERE false; bila ada cabang yang bernilai TRUE tanpa
// batasan (hanya AllRows) tidak ada klausa yang ditambahkan.
func applyRowFilter(ctx context.Context, db *gorm.DB, rf RowFilter) *gorm.DB {
	var branches []clause.Expression
	for _, b := range rf.Branches {
		expr, state := branchExpr(ctx, db, b)
		switch state {
		case branchTrue:
			return db
		case branchExprState:
			branches = append(branches, expr)
		}
	}
	switch len(branches) {
	case 0:
		return db.Where(falseExpr)
	case 1:
		return db.Where(branches[0])
	}
	return db.Where(orExpr(branches))
}

type branchState uint8

const (
	branchFalse     branchState = iota // cabang bernilai FALSE (dilepas)
	branchTrue                         // cabang bernilai TRUE tanpa batasan
	branchExprState                    // cabang membatasi lewat ekspresi
)

func branchExpr(ctx context.Context, db *gorm.DB, b RowBranch) (clause.Expression, branchState) {
	if len(b.Conditions) == 0 {
		return nil, branchFalse
	}
	var exprs []clause.Expression
	for _, c := range b.Conditions {
		expr, state := conditionExpr(ctx, db, c)
		switch state {
		case branchFalse:
			return nil, branchFalse
		case branchExprState:
			exprs = append(exprs, expr)
		}
	}
	switch len(exprs) {
	case 0:
		return nil, branchTrue
	case 1:
		return exprs[0], branchExprState
	}
	return andExpr(exprs), branchExprState
}

func conditionExpr(ctx context.Context, db *gorm.DB, c RowCondition) (clause.Expression, branchState) {
	switch c.Kind {
	case CondAll:
		return nil, branchTrue
	case CondOwner:
		if !validIdent(c.Column) || len(c.Values) != 1 || c.Values[0] == "" {
			return nil, branchFalse
		}
		return clause.Eq{Column: clause.Column{Table: clause.CurrentTable, Name: c.Column}, Value: c.Values[0]}, branchExprState
	case CondAssigned:
		if !validIdent(c.Column) || len(c.Values) == 0 {
			return nil, branchFalse
		}
		vals := make([]any, len(c.Values))
		for i, v := range c.Values {
			vals[i] = v
		}
		return clause.IN{Column: clause.Column{Table: clause.CurrentTable, Name: c.Column}, Values: vals}, branchExprState
	case CondCustom:
		return customExpr(ctx, db, c.Provider)
	}
	return nil, branchFalse
}

// customExpr menjalankan ScopeProvider.Filter pada sesi bersih lalu mengambil kondisi WHERE-nya sebagai
// satu kelompok. Panic, nil, atau hasil tanpa kondisi WHERE bernilai FALSE.
func customExpr(ctx context.Context, db *gorm.DB, p ScopeProvider) (expr clause.Expression, state branchState) {
	if p.Filter == nil {
		return nil, branchFalse
	}
	defer func() {
		if r := recover(); r != nil {
			slog.ErrorContext(ctx, "authz: panic pada ScopeProvider.Filter", "scope", p.Key, "panic", r)
			expr, state = nil, branchFalse
		}
	}()
	sub := p.Filter(ctx, db.Session(&gorm.Session{NewDB: true}))
	if sub == nil || sub.Statement == nil {
		return nil, branchFalse
	}
	c, ok := sub.Statement.Clauses["WHERE"]
	if !ok {
		return nil, branchFalse
	}
	where, ok := c.Expression.(clause.Where)
	if !ok || len(where.Exprs) == 0 {
		return nil, branchFalse
	}
	exprs := append([]clause.Expression(nil), where.Exprs...)
	// Meniru penanganan kelompok kondisi GORM: satu ekspresi OR tunggal dibungkus sebagai AND agar tidak
	// membocorkan konektor OR ke luar kelompok.
	if len(exprs) == 1 {
		if oc, ok := exprs[0].(clause.OrConditions); ok {
			exprs[0] = clause.AndConditions(oc)
		}
	}
	return clause.And(exprs...), branchExprState
}

// andExpr dan orExpr membangun "(a AND b)" dan "(a OR b)" secara eksplisit sehingga pengelompokan tidak
// bergantung pada perilaku penggabung klausa GORM.
type andExpr []clause.Expression

func (a andExpr) Build(b clause.Builder) { buildJoined(b, " AND ", a) }

type orExpr []clause.Expression

func (o orExpr) Build(b clause.Builder) { buildJoined(b, " OR ", o) }

func buildJoined(b clause.Builder, sep string, exprs []clause.Expression) {
	_ = b.WriteByte('(')
	for i, e := range exprs {
		if i > 0 {
			_, _ = b.WriteString(sep)
		}
		e.Build(b)
	}
	_ = b.WriteByte(')')
}
