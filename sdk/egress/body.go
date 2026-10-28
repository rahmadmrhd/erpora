package egress

import "io"

// limitedBody membatasi jumlah byte body respons; melewati batas mengembalikan ErrResponseTooLarge
// (tidak memotong diam-diam).
type limitedBody struct {
	rc        io.ReadCloser
	remaining int64
	exceeded  bool
}

func (b *limitedBody) Read(p []byte) (int, error) {
	if b.exceeded {
		return 0, ErrResponseTooLarge
	}
	if len(p) == 0 {
		return 0, nil
	}
	if int64(len(p)) > b.remaining+1 {
		p = p[:b.remaining+1] // baca satu byte lebih untuk mendeteksi kelebihan
	}
	n, err := b.rc.Read(p)
	if int64(n) > b.remaining {
		n = int(b.remaining)
		b.remaining = 0
		b.exceeded = true
		return n, ErrResponseTooLarge
	}
	b.remaining -= int64(n)
	return n, err
}

func (b *limitedBody) Close() error { return b.rc.Close() }
