package cli

import "testing"

// TestDeviceInstructionsPreferTheFilledInLink: Logto sends
// verification_uri_complete, so the page opens with the code in it and the
// printed code is only there to compare.
func TestDeviceInstructionsPreferTheFilledInLink(t *testing.T) {
	got := deviceInstructions(deviceAuthResponse{
		UserCode:                "ZFKL-KTPM",
		VerificationURI:         "https://accounts.example/device",
		VerificationURIComplete: "https://accounts.example/device?user_code=ZFKL-KTPM",
	})
	want := "Open https://accounts.example/device?user_code=ZFKL-KTPM\nThe page shows code ZFKL-KTPM; check it matches, then sign in.\nWaiting..."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	got = deviceInstructions(deviceAuthResponse{UserCode: "ABCD-EFGH", VerificationURI: "https://accounts.example/device"})
	want = "Open https://accounts.example/device and enter code ABCD-EFGH, then sign in.\nWaiting..."
	if got != want {
		t.Errorf("without the complete link: got %q, want %q", got, want)
	}
}
