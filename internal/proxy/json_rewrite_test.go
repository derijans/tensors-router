package proxy

import "testing"

func TestRewriteJSONModelReplacesOnlyTopLevelModelKeys(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "string value equal to the field name", body: `{"object":"model","model":"backend-local"}`, want: `{"object":"model","model":"public"}`},
		{name: "duplicate keys", body: `{"model":"first","model":"second"}`, want: `{"model":"public","model":"public"}`},
		{name: "escaped key", body: `{"mod\u0065l":"backend-local"}`, want: `{"mod\u0065l":"public"}`},
		{name: "nested model untouched", body: `{"choices":[{"model":"nested"}],"model":"top"}`, want: `{"choices":[{"model":"nested"}],"model":"public"}`},
		{name: "non-string model untouched", body: `{"model":null}`, want: `{"model":null}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := string(rewriteJSONModel([]byte(test.body), "public")); got != test.want {
				t.Fatalf("rewriteJSONModel(%s) = %s, want %s", test.body, got, test.want)
			}
		})
	}
}
