package guard

import "container/list"

// lruMap adalah peta berurutan akses terakhir dengan batas jumlah entri: get memindahkan entri ke depan, add menggusur
// entri paling lama tak diakses bila melebihi batas. Urutan daftar identik dengan urutan waktu akses terakhir, sehingga
// entri kedaluwarsa selalu berada di belakang dan dapat dibuang satu per satu dari ujung (O(1) teramortisasi). Tidak
// aman untuk goroutine; pemanggil memegang kuncinya.
type lruMap[V any] struct {
	ll  *list.List
	m   map[string]*list.Element
	max int
}

type lruEntry[V any] struct {
	key string
	val V
}

func newLRU[V any](maxEntries int) *lruMap[V] {
	return &lruMap[V]{ll: list.New(), m: map[string]*list.Element{}, max: maxEntries}
}

// get mengembalikan nilai untuk key dan menandainya sebagai yang terbaru.
func (c *lruMap[V]) get(key string) (*V, bool) {
	el, ok := c.m[key]
	if !ok {
		return nil, false
	}
	c.ll.MoveToFront(el)
	return &el.Value.(*lruEntry[V]).val, true
}

// add menyimpan v untuk key (key belum ada) sebagai yang terbaru dan menggusur yang tertua bila melebihi batas.
func (c *lruMap[V]) add(key string, v V) *V {
	e := &lruEntry[V]{key: key, val: v}
	c.m[key] = c.ll.PushFront(e)
	for c.max > 0 && c.ll.Len() > c.max {
		c.removeOldest()
	}
	return &e.val
}

// oldest mengembalikan entri paling lama tak diakses.
func (c *lruMap[V]) oldest() (*V, bool) {
	el := c.ll.Back()
	if el == nil {
		return nil, false
	}
	return &el.Value.(*lruEntry[V]).val, true
}

func (c *lruMap[V]) removeOldest() {
	el := c.ll.Back()
	if el == nil {
		return
	}
	delete(c.m, el.Value.(*lruEntry[V]).key)
	c.ll.Remove(el)
}

func (c *lruMap[V]) len() int { return c.ll.Len() }
