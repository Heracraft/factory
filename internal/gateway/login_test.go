package gateway

import "testing"

func TestParseLogin(t *testing.T) {
	cases := []struct {
		in           string
		slug, handle string
		ok           bool
	}{
		{"todo-app.heracraft", "todo-app", "heracraft", true},
		{"a.b", "a", "b", true},
		{"my.dotted.name.user1", "my.dotted.name", "", false},
		{"todo-app", "", "", false},
		{".heracraft", "", "", false},
		{"todo-app.", "", "", false},
		{"", "", "", false},
		{"Todo-App.heracraft", "", "", false},
		{"todo app.heracraft", "", "", false},
		{"todo-app.hera craft", "", "", false},
	}
	for _, c := range cases {
		slug, handle, err := ParseLogin(c.in)
		if c.ok != (err == nil) {
			t.Errorf("%q: ok=%v err=%v", c.in, c.ok, err)
			continue
		}
		if c.ok && (slug != c.slug || handle != c.handle) {
			t.Errorf("%q: got %q %q", c.in, slug, handle)
		}
		if !c.ok && err.Error() != BadLoginMessage {
			t.Errorf("%q: error text %q", c.in, err)
		}
	}
}
