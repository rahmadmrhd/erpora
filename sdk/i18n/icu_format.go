package i18n

import (
	"fmt"
	"strings"

	"golang.org/x/text/language"
)

// formatter menyimpan keadaan satu panggilan Format.
type formatter struct {
	loc    Locale
	tag    language.Tag
	tagSet bool
	args   Args
	sb     strings.Builder
}

func (f *formatter) langTag() language.Tag {
	if !f.tagSet {
		f.tag = language.Make(string(f.loc))
		f.tagSet = true
	}
	return f.tag
}

func (f *formatter) write(s string) error {
	if f.sb.Len()+len(s) > MaxFormatOutput {
		return ErrOutputTooLarge
	}
	f.sb.WriteString(s)
	return nil
}

// arg mengembalikan nilai argumen dan false bila hilang (tidak ada atau nil).
func (f *formatter) arg(name string) (any, bool) {
	v, ok := f.args[name]
	return v, ok && v != nil
}

// Format mengimplementasi Message. Tidak pernah panik: panic internal dikembalikan sebagai galat.
func (m *message) Format(loc Locale, args Args) (out string, err error) {
	defer func() {
		if r := recover(); r != nil {
			out, err = "", fmt.Errorf("i18n: panic saat Format (%T)", r)
		}
	}()
	f := &formatter{loc: loc, args: args}
	if err := f.nodes(m.nodes, ""); err != nil {
		return "", err
	}
	return f.sb.String(), nil
}

// nodes menulis simpul; pound adalah teks '#' untuk plural terdekat.
func (f *formatter) nodes(ns []node, pound string) error {
	for _, n := range ns {
		switch x := n.(type) {
		case textNode:
			if err := f.write(string(x)); err != nil {
				return err
			}
		case poundNode:
			if err := f.write(pound); err != nil {
				return err
			}
		case argNode:
			v, ok := f.arg(x.name)
			if !ok {
				if err := f.write("{" + x.name + "}"); err != nil {
					return err
				}
				continue
			}
			s, err := argString(v)
			if err != nil {
				return fmt.Errorf("argumen %q: %w", x.name, err)
			}
			if err := f.write(s); err != nil {
				return err
			}
		case tagNode:
			if err := f.nodes(x.body, pound); err != nil {
				return err
			}
		case blockNode:
			if err := f.block(x); err != nil {
				return err
			}
		}
	}
	return nil
}

func (f *formatter) block(b blockNode) error {
	v, ok := f.arg(b.name)
	if !ok {
		return f.write("{" + b.name + "}")
	}
	sv, err := argString(v)
	if err != nil {
		return fmt.Errorf("argumen %q: %w", b.name, err)
	}
	var chosen *option
	var pd *decimal // bilangan plural untuk '#'
	if b.plural {
		num, err := argNumber(v)
		if err != nil {
			return fmt.Errorf("argumen %q: %w", b.name, err)
		}
		for i := range b.opts {
			if b.opts[i].exact && b.opts[i].key == "="+sv {
				chosen = &b.opts[i]
				break
			}
		}
		d := newDecimal(num)
		if chosen == nil {
			form := d.form(f.langTag())
			for i := range b.opts {
				if !b.opts[i].exact && b.opts[i].cat == form {
					chosen = &b.opts[i]
					break
				}
			}
		}
		pd = &d
	} else {
		for i := range b.opts {
			if b.opts[i].key == sv {
				chosen = &b.opts[i]
				break
			}
		}
	}
	if chosen == nil {
		for i := range b.opts {
			if b.opts[i].key == "other" {
				chosen = &b.opts[i]
				break
			}
		}
	}
	if chosen == nil {
		return nil // tidak terjadi: Compile mewajibkan 'other'
	}
	// '#' hanya berlaku langsung di cabang plural; select di dalamnya memperlakukannya sebagai literal
	// (parser sudah memisahkannya), jadi pound hanya diteruskan ke cabang plural ini.
	pound := ""
	if pd != nil && chosen.pound {
		pound = pd.format(f.langTag())
	}
	return f.nodes(chosen.body, pound)
}
