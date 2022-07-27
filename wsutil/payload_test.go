package wsutil

import "testing"

func TestObjectPayload_String(t *testing.T) {
	tests := []struct {
		name string
		p    ObjectPayload
		key  string
		want string
	}{
		{
			name: "exact match",
			p: map[string]interface{}{
				"foo": "bar",
			},
			key:  "foo",
			want: "bar",
		},
		{
			name: "case-insensitive match",
			p: map[string]interface{}{
				"foo": "bar",
			},
			key:  "Foo",
			want: "bar",
		},
		{
			name: "no match",
			p:    nil,
			key:  "foo",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.String(tt.key); got != tt.want {
				t.Errorf("ObjectPayload.String() = %v, want %v", got, tt.want)
			}
		})
	}
}
