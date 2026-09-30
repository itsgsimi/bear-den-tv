// Tests that the app install IPC messages and apps.browser (install.go, contracts/ipc.md)
// decode to their typed values and round-trip through Encode.

package shellipc

import (
	"reflect"
	"testing"
)

func TestInstallMessagesDecode(t *testing.T) {
	cases := map[string]Message{
		`{"type":"app.install","request_id":"r1","app_id":"moonlight"}`:                            AppInstall{Type: TypeAppInstall, RequestID: "r1", AppID: "moonlight"},
		`{"type":"app.install_info","request_id":"r2","app_id":"netflix"}`:                         AppInstallInfo{Type: TypeAppInstallInfo, RequestID: "r2", AppID: "netflix"},
		`{"type":"app.install_cancel","request_id":"r3","app_id":"moonlight"}`:                     AppInstallCancel{Type: TypeAppInstallCancel, RequestID: "r3", AppID: "moonlight"},
		`{"type":"apps.configure","request_id":"r4","auto_update":true}`:                           AppsConfigure{Type: TypeAppsConfigure, RequestID: "r4", AutoUpdate: true},
		`{"type":"apps.browser","request_id":"r6","browser":"brave","streaming_browser":"chrome"}`: AppsBrowser{Type: TypeAppsBrowser, RequestID: "r6", Browser: "brave", StreamingBrowser: "chrome"},
		// Install pressed on the app's own card: turn it on once installed.
		`{"type":"app.install","request_id":"r7","app_id":"netflix","enable":true}`: AppInstall{Type: TypeAppInstall, RequestID: "r7", AppID: "netflix", Enable: true},
		// Remove, with the app's data.
		`{"type":"app.uninstall","request_id":"r8","app_id":"moonlight","delete_data":true}`: AppUninstall{Type: TypeAppUninstall, RequestID: "r8", AppID: "moonlight", DeleteData: true},
		// The older reserved name is an alias of app.install.
		`{"type":"applications.install_request","request_id":"r5","app_id":"spotify"}`: InstallRequest{Type: TypeInstallRequest, RequestID: "r5", AppID: "spotify"},
	}
	for frame, want := range cases {
		got, err := Decode([]byte(frame))
		if err != nil {
			t.Fatalf("%s: %v", frame, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: got %#v, want %#v", frame, got, want)
		}
		enc, err := Encode(got)
		if err != nil {
			t.Fatal(err)
		}
		again, err := Decode(enc[:len(enc)-1])
		if err != nil || !reflect.DeepEqual(again, want) {
			t.Fatalf("%s: round trip gave %#v, %v", frame, again, err)
		}
	}
}
