package funcs

import (
	"html/template"
	"net/url"
	"testing"
	"time"

	"github.com/sqlwarden/internal/assert"
)

func TestFormatTime(t *testing.T) {
	testTime := time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC)

	tests := []struct {
		name     string
		format   string
		expected string
	}{
		{"RFC3339 format", time.RFC3339, "2024-03-15T14:30:00Z"},
		{"Custom format", "2006-01-02 15:04", "2024-03-15 14:30"},
		{"Date only", "2006-01-02", "2024-03-15"},
		{"Time only", "15:04:05", "14:30:00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTime(tt.format, testTime)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestApproxDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{"One year", 365 * 24 * time.Hour, "1 year"},
		{"Two years", 2 * 365 * 24 * time.Hour, "2 years"},
		{"One day", 24 * time.Hour, "1 day"},
		{"Multiple days", 5 * 24 * time.Hour, "5 days"},
		{"One hour", time.Hour, "1 hour"},
		{"Multiple hours", 3 * time.Hour, "3 hours"},
		{"One minute", time.Minute, "1 minute"},
		{"Multiple minutes", 45 * time.Minute, "45 minutes"},
		{"One second", time.Second, "1 second"},
		{"Multiple seconds", 30 * time.Second, "30 seconds"},
		{"Sub-second", 500 * time.Millisecond, "less than 1 second"},
		{"Zero duration", 0, "less than 1 second"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := approxDuration(tt.duration)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPluralize(t *testing.T) {
	tests := []struct {
		name      string
		count     any
		singular  string
		plural    string
		expected  string
		shouldErr bool
	}{
		{"Count of 1", 1, "item", "items", "item", false},
		{"Count of 0", 0, "item", "items", "items", false},
		{"Count greater than 1", 5, "item", "items", "items", false},
		{"Negative count", -1, "item", "items", "items", false},
		{"String number 1", "1", "item", "items", "item", false},
		{"String number 0", "0", "item", "items", "items", false},
		{"String number greater than 1", "5", "item", "items", "items", false},
		{"Invalid string", "invalid", "item", "items", "", true},
		{"Float type", 3.14, "item", "items", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := pluralize(tt.count, tt.singular, tt.plural)
			if tt.shouldErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Spaces to hyphens", "hello world", "hello-world"},
		{"Uppercase to lowercase", "Hello World", "hello-world"},
		{"Preserve hyphens and underscores", "hello-world_test", "hello-world_test"},
		{"Remove special characters", "hello@world#test!", "helloworldtest"},
		{"Remove non-ASCII characters", "héllo wørld", "hllo-wrld"},
		{"Empty string", "", ""},
		{"Numbers preserved", "hello123world", "hello123world"},
		{"Multiple spaces", "hello   world", "hello---world"},
		{"Mixed case with numbers", "Hello123-World_Test", "hello123-world_test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := slugify(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSafeHTML(t *testing.T) {
	t.Run("Returns template HTML type", func(t *testing.T) {
		input := "<b>Hello World</b>"
		result := safeHTML(input)

		assert.Equal(t, template.HTML(input), result)
	})
}

func TestIncr(t *testing.T) {
	tests := []struct {
		name      string
		input     any
		expected  int64
		shouldErr bool
	}{
		{"Integer", 5, 6, false},
		{"Zero", 0, 1, false},
		{"Negative", -1, 0, false},
		{"String number", "10", 11, false},
		{"String zero", "0", 1, false},
		{"Invalid string", "invalid", 0, true},
		{"Float type", 3.14, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := incr(tt.input)
			if tt.shouldErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDecr(t *testing.T) {
	tests := []struct {
		name      string
		input     any
		expected  int64
		shouldErr bool
	}{
		{"Integer", 5, 4, false},
		{"Zero", 0, -1, false},
		{"Negative", -1, -2, false},
		{"String number", "10", 9, false},
		{"String zero", "0", -1, false},
		{"Invalid string", "invalid", 0, true},
		{"Float type", 3.14, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := decr(tt.input)
			if tt.shouldErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatInt(t *testing.T) {
	tests := []struct {
		name      string
		input     any
		expected  string
		shouldErr bool
	}{
		{"Large number with separators", 1234567, "1,234,567", false},
		{"Small number", 42, "42", false},
		{"Zero", 0, "0", false},
		{"Negative number", -1000, "-1,000", false},
		{"String number", "1000", "1,000", false},
		{"Invalid string", "invalid", "", true},
		{"Float type", 3.14, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := formatInt(tt.input)
			if tt.shouldErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatFloat(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		dp       int
		expected string
	}{
		{"Two decimal places", 3.14159, 2, "3.14"},
		{"Zero decimal places", 123.456, 0, "123"},
		{"Large number with separators", 1234.5678, 2, "1,234.57"},
		{"Negative number", -123.456, 1, "-123.5"},
		{"Zero value", 0.0, 2, "0.00"},
		{"Many decimal places", 3.14159, 4, "3.1416"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatFloat(tt.input, tt.dp)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestYesNo(t *testing.T) {
	tests := []struct {
		name     string
		input    bool
		expected string
	}{
		{"True value", true, "Yes"},
		{"False value", false, "No"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := yesNo(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUrlSetParam(t *testing.T) {
	t.Run("Sets new parameter", func(t *testing.T) {
		u, _ := url.Parse("https://example.com/path")

		result := urlSetParam(u, "foo", "bar")

		assert.Equal(t, "foo=bar", result.RawQuery)
		assert.Equal(t, "https://example.com/path", result.Scheme+"://"+result.Host+result.Path)
	})

	t.Run("Updates existing parameter", func(t *testing.T) {
		u, _ := url.Parse("https://example.com/path?foo=old")

		result := urlSetParam(u, "foo", "new")

		assert.Equal(t, "foo=new", result.RawQuery)
	})

	t.Run("Preserves existing parameters", func(t *testing.T) {
		u, _ := url.Parse("https://example.com/path?existing=value")

		result := urlSetParam(u, "new", "param")

		values := result.Query()
		assert.Equal(t, "value", values.Get("existing"))
		assert.Equal(t, "param", values.Get("new"))
	})

	t.Run("Does not modify original URL", func(t *testing.T) {
		u, _ := url.Parse("https://example.com/path")
		originalQuery := u.RawQuery

		urlSetParam(u, "foo", "bar")

		assert.Equal(t, originalQuery, u.RawQuery)
	})
}

func TestUrlDelParam(t *testing.T) {
	t.Run("Deletes existing parameter", func(t *testing.T) {
		u, _ := url.Parse("https://example.com/path?foo=bar&baz=qux")

		result := urlDelParam(u, "foo")

		assert.Equal(t, "baz=qux", result.RawQuery)
	})

	t.Run("Preserves other parameters", func(t *testing.T) {
		u, _ := url.Parse("https://example.com/path?foo=bar&baz=qux")

		result := urlDelParam(u, "foo")

		values := result.Query()
		assert.Equal(t, "", values.Get("foo"))
		assert.Equal(t, "qux", values.Get("baz"))
	})

	t.Run("Does nothing for non-existent parameter", func(t *testing.T) {
		u, _ := url.Parse("https://example.com/path?foo=bar")

		result := urlDelParam(u, "nonexistent")

		assert.Equal(t, "foo=bar", result.RawQuery)
	})

	t.Run("Does not modify original URL", func(t *testing.T) {
		u, _ := url.Parse("https://example.com/path?foo=bar")
		originalQuery := u.RawQuery

		urlDelParam(u, "foo")

		assert.Equal(t, originalQuery, u.RawQuery)
	})
}

func TestToInt64(t *testing.T) {
	tests := []struct {
		name      string
		input     any
		expected  int64
		shouldErr bool
	}{
		{"int", int(42), 42, false},
		{"int8", int8(42), 42, false},
		{"int16", int16(42), 42, false},
		{"int32", int32(42), 42, false},
		{"int64", int64(42), 42, false},
		{"uint", uint(42), 42, false},
		{"uint8", uint8(42), 42, false},
		{"uint16", uint16(42), 42, false},
		{"uint32", uint32(42), 42, false},
		{"string number", "12345", 12345, false},
		{"string negative", "-123", -123, false},
		{"invalid string", "invalid", 0, true},
		{"empty string", "", 0, true},
		{"float64", 3.14, 0, true},
		{"bool", true, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := toInt64(tt.input)
			if tt.shouldErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
			assert.Equal(t, tt.expected, result)
		})
	}
}
