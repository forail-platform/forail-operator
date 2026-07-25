package main

import (
	"reflect"
	"testing"
)

func TestSecretCacheNamespaces(t *testing.T) {
	tests := []struct {
		name  string
		own   string
		extra string
		want  []string
	}{
		{
			name: "own namespace only",
			own:  "forail-operator",
			want: []string{"forail-operator"},
		},
		{
			name:  "extras are appended in order",
			own:   "forail-operator",
			extra: "team-a,team-b",
			want:  []string{"forail-operator", "team-a", "team-b"},
		},
		{
			name:  "whitespace and empty entries are dropped",
			own:   "forail-operator",
			extra: " team-a , , team-b ,",
			want:  []string{"forail-operator", "team-a", "team-b"},
		},
		{
			// The chart always renders a Role for the release namespace, so a
			// user repeating it must not produce a duplicate cache entry.
			name:  "own namespace repeated in extras is deduplicated",
			own:   "forail-operator",
			extra: "team-a,forail-operator,team-a",
			want:  []string{"forail-operator", "team-a"},
		},
		{
			// Out-of-cluster (`make run`): nil means "do not scope the cache",
			// which is what the unscoped kubeconfig path expects.
			name:  "unknown own namespace is unscoped even with extras",
			own:   "",
			extra: "team-a",
			want:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := secretCacheNamespaces(tc.own, tc.extra)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("secretCacheNamespaces(%q, %q) = %v, want %v", tc.own, tc.extra, got, tc.want)
			}
		})
	}
}
