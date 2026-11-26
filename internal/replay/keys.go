// Reads the keys pressed during playback.

package replay

import "io"

func Keys(r io.Reader) <-chan byte {
	ch := make(chan byte, 16)
	go func() {
		defer close(ch)
		var tk tokenizer
		buf := make([]byte, 64)
		for {
			n, err := r.Read(buf)
			tk.Feed(buf[:n], func(t token) {
				if t.kind == tokPrint || t.kind == tokControl {
					for _, b := range t.raw {
						select {
						case ch <- b:
						default:
						}
					}
				}
			})
			if err != nil {
				return
			}
		}
	}()
	return ch
}
