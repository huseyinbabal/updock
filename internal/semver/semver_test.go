package semver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	tests := []struct {
		tag     string
		wantErr bool
		major   int
		minor   int
		patch   int
		pre     string
	}{
		{"1.2.3", false, 1, 2, 3, ""},
		{"v1.2.3", false, 1, 2, 3, ""},
		{"8.0.45", false, 8, 0, 45, ""},
		{"1.25", false, 1, 25, 0, ""},
		{"3", false, 3, 0, 0, ""},
		{"8.0.45-bookworm", false, 8, 0, 45, "bookworm"},
		{"3-alpine", false, 3, 0, 0, "alpine"},
		{"1.2.3-rc1", false, 1, 2, 3, "rc1"},
		{"1.2.3+build", false, 1, 2, 3, ""},
		{"v2.0.0-beta.1", false, 2, 0, 0, "beta.1"},
		{"latest", true, 0, 0, 0, ""},
		{"alpine", true, 0, 0, 0, ""},
		{"bookworm", true, 0, 0, 0, ""},
		{"", true, 0, 0, 0, ""},
		{"lts", true, 0, 0, 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			v, err := Parse(tt.tag)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.major, v.Major)
			assert.Equal(t, tt.minor, v.Minor)
			assert.Equal(t, tt.patch, v.Patch)
			assert.Equal(t, tt.pre, v.PreRelease)
			assert.Equal(t, tt.tag, v.String())
		})
	}
}

func TestCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.1", "1.0.0", 1},
		{"1.0.0", "1.0.1", -1},
		{"1.1.0", "1.0.9", 1},
		{"2.0.0", "1.9.9", 1},
		{"1.0.0", "1.0.0-rc1", 1},
		{"1.0.0-rc1", "1.0.0", -1},
		{"1.0.0-alpha", "1.0.0-beta", -1},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			a, _ := Parse(tt.a)
			b, _ := Parse(tt.b)
			assert.Equal(t, tt.want, a.Compare(b))
		})
	}
}

func TestIsPatchUpdate(t *testing.T) {
	base, _ := Parse("8.0.45")
	newer, _ := Parse("8.0.46")
	minor, _ := Parse("8.1.0")
	major, _ := Parse("9.0.0")

	assert.True(t, newer.IsPatchUpdate(base))
	assert.False(t, minor.IsPatchUpdate(base))
	assert.False(t, major.IsPatchUpdate(base))
	assert.False(t, base.IsPatchUpdate(base))
}

func TestIsMinorUpdate(t *testing.T) {
	base, _ := Parse("1.25.0")
	patch, _ := Parse("1.25.1")
	minor, _ := Parse("1.26.0")
	major, _ := Parse("2.0.0")

	assert.True(t, patch.IsMinorUpdate(base))
	assert.True(t, minor.IsMinorUpdate(base))
	assert.False(t, major.IsMinorUpdate(base))
}

func TestHasSamePreReleaseSuffix(t *testing.T) {
	a, _ := Parse("8.0.45-bookworm")
	b, _ := Parse("8.0.46-bookworm")
	c, _ := Parse("8.0.45-alpine")
	d, _ := Parse("8.0.45")

	assert.True(t, a.HasSamePreReleaseSuffix(b))
	assert.False(t, a.HasSamePreReleaseSuffix(c))
	assert.False(t, a.HasSamePreReleaseSuffix(d))
	assert.True(t, d.HasSamePreReleaseSuffix(d))
}

func TestFilterByStrategy(t *testing.T) {
	current, _ := Parse("8.0.45")
	candidates := []Version{}
	for _, tag := range []string{"7.0.0", "8.0.44", "8.0.45", "8.0.46", "8.0.47", "8.1.0", "8.2.0", "9.0.0"} {
		v, _ := Parse(tag)
		candidates = append(candidates, v)
	}

	// Patch: should pick 8.0.47 (highest patch in 8.0.x)
	best := FilterByStrategy(current, candidates, "patch")
	require.NotNil(t, best)
	assert.Equal(t, 8, best.Major)
	assert.Equal(t, 0, best.Minor)
	assert.Equal(t, 47, best.Patch)

	// Minor: should pick 8.2.0 (highest in 8.x.x)
	best = FilterByStrategy(current, candidates, "minor")
	require.NotNil(t, best)
	assert.Equal(t, 8, best.Major)
	assert.Equal(t, 2, best.Minor)

	// Major: should pick 9.0.0
	best = FilterByStrategy(current, candidates, "major")
	require.NotNil(t, best)
	assert.Equal(t, 9, best.Major)

	// Digest/Pin: should return nil
	assert.Nil(t, FilterByStrategy(current, candidates, "digest"))
	assert.Nil(t, FilterByStrategy(current, candidates, "pin"))
	assert.Nil(t, FilterByStrategy(current, candidates, ""))
}

func TestFilterByStrategy_WithPreRelease(t *testing.T) {
	current, _ := Parse("8.0.45-bookworm")
	candidates := []Version{}
	for _, tag := range []string{"8.0.46-bookworm", "8.0.46-alpine", "8.0.46", "8.0.47-bookworm"} {
		v, _ := Parse(tag)
		candidates = append(candidates, v)
	}

	// Should only pick bookworm variants
	best := FilterByStrategy(current, candidates, "patch")
	require.NotNil(t, best)
	assert.Equal(t, "8.0.47-bookworm", best.Original)
}

func TestFilterByStrategy_NoCandidates(t *testing.T) {
	current, _ := Parse("9.0.0")
	candidates := []Version{}
	for _, tag := range []string{"8.0.0", "7.0.0"} {
		v, _ := Parse(tag)
		candidates = append(candidates, v)
	}

	assert.Nil(t, FilterByStrategy(current, candidates, "patch"))
	assert.Nil(t, FilterByStrategy(current, candidates, "major"))
}
