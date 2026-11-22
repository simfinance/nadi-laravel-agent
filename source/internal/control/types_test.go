package control

import (
	"encoding/json"
	"testing"
)

func TestEndpointUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string
	}{
		{
			name: "URL string",
			json: `"https://sikasir.pesantrenalirsyad.org/_nadi/v1"`,
			want: "https://sikasir.pesantrenalirsyad.org/_nadi/v1",
		},
		{
			name: "object with base URL",
			json: `{"base_url":"https://sikasir.pesantrenalirsyad.org/_nadi/v1"}`,
			want: "https://sikasir.pesantrenalirsyad.org/_nadi/v1",
		},
		{
			name: "null",
			json: `null`,
			want: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var endpoint Endpoint
			if err := json.Unmarshal([]byte(test.json), &endpoint); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}

			if endpoint.BaseURL != test.want {
				t.Fatalf("BaseURL = %q, want %q", endpoint.BaseURL, test.want)
			}
		})
	}
}
