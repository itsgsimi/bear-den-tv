// QRModules: encodes pairing URLs as QR module rows for the shell (spec
// contracts/state.schema.json).

package pairing

import (
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

// QRModules encodes content as a QR code and returns one string per row of
// modules, "1" for dark and "0" for light, including the quiet zone. The
// shell renders the rows; no image crosses the IPC socket.
func QRModules(content string) ([]string, error) {
	q, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		return nil, err
	}
	bitmap := q.Bitmap()
	rows := make([]string, len(bitmap))
	for i, row := range bitmap {
		var b strings.Builder
		b.Grow(len(row))
		for _, dark := range row {
			if dark {
				b.WriteByte('1')
			} else {
				b.WriteByte('0')
			}
		}
		rows[i] = b.String()
	}
	return rows, nil
}
