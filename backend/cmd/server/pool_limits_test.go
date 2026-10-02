package main

import "testing"

func TestBoundedPoolConnectionCount(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		value   int
		minimum int32
		maximum int32
		want    int32
		wantErr bool
	}{
		{name: "accepts lower bound", value: 1, minimum: 1, maximum: 100, want: 1},
		{name: "accepts maximum", value: 100, minimum: 1, maximum: 100, want: 100},
		{name: "caps oversized value", value: 1000, minimum: 1, maximum: 100, want: 100},
		{name: "rejects negative idle count", value: -1, minimum: 0, maximum: 50, wantErr: true},
		{name: "rejects zero open count", value: 0, minimum: 1, maximum: 100, wantErr: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := boundedPoolConnectionCount(testCase.value, testCase.minimum, testCase.maximum)
			if testCase.wantErr {
				if err == nil {
					t.Fatal("boundedPoolConnectionCount() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("boundedPoolConnectionCount() error = %v", err)
			}
			if got != testCase.want {
				t.Fatalf("boundedPoolConnectionCount() = %d, want %d", got, testCase.want)
			}
		})
	}
}
