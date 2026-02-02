package funcs

import (
	"bytes"
	"fmt"
	"html/template"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

var printer = message.NewPrinter(language.English)

var TemplateFuncs = template.FuncMap{

	"now":            time.Now,
	"timeSince":      time.Since,
	"timeUntil":      time.Until,
	"formatTime":     formatTime,
	"approxDuration": approxDuration,

	"uppercase": strings.ToUpper,
	"lowercase": strings.ToLower,
	"pluralize": pluralize,
	"slugify":   slugify,
	"safeHTML":  safeHTML,

	"join": strings.Join,

	"incr":        incr,
	"decr":        decr,
	"formatInt":   formatInt,
	"formatFloat": formatFloat,

	"yesNo": yesNo,

	"urlSetParam": urlSetParam,
	"urlDelParam": urlDelParam,
}

func formatTime(format string, t time.Time) string {
	return t.Format(format)
}

func approxDuration(d time.Duration) string {
	const (
		day  = 24 * time.Hour
		year = 365 * day
	)

	formatUnit := func(count int, singular, plural string) string {
		if count == 1 {
			return fmt.Sprintf("1 %s", singular)
		}
		return fmt.Sprintf("%d %s", count, plural)
	}

	switch {
	case d >= year:
		return formatUnit(int(math.Round(float64(d)/float64(year))), "year", "years")
	case d >= day:
		return formatUnit(int(math.Round(float64(d)/float64(day))), "day", "days")
	case d >= time.Hour:
		return formatUnit(int(math.Round(d.Hours())), "hour", "hours")
	case d >= time.Minute:
		return formatUnit(int(math.Round(d.Minutes())), "minute", "minutes")
	case d >= time.Second:
		return formatUnit(int(math.Round(d.Seconds())), "second", "seconds")
	default:
		return "less than 1 second"
	}
}

func pluralize(count any, singular string, plural string) (string, error) {
	n, err := toInt64(count)
	if err != nil {
		return "", err
	}

	if n == 1 {
		return singular, nil
	}

	return plural, nil
}

func slugify(s string) string {
	var buf bytes.Buffer

	for _, r := range s {
		switch {
		case r > unicode.MaxASCII:
			continue
		case unicode.IsLetter(r):
			buf.WriteRune(unicode.ToLower(r))
		case unicode.IsDigit(r), r == '_', r == '-':
			buf.WriteRune(r)
		case unicode.IsSpace(r):
			buf.WriteRune('-')
		}
	}

	return buf.String()
}

func safeHTML(s string) template.HTML {
	return template.HTML(s)
}

func incr(i any) (int64, error) {
	n, err := toInt64(i)
	if err != nil {
		return 0, err
	}

	n++
	return n, nil
}

func decr(i any) (int64, error) {
	n, err := toInt64(i)
	if err != nil {
		return 0, err
	}

	n--
	return n, nil
}

func formatInt(i any) (string, error) {
	n, err := toInt64(i)
	if err != nil {
		return "", err
	}

	return printer.Sprintf("%d", n), nil
}

func formatFloat(f float64, dp int) string {
	format := "%." + strconv.Itoa(dp) + "f"
	return printer.Sprintf(format, f)
}

func yesNo(b bool) string {
	if b {
		return "Yes"
	}

	return "No"
}

func urlSetParam(u *url.URL, key string, value any) *url.URL {
	nu := *u
	values := nu.Query()

	values.Set(key, fmt.Sprintf("%v", value))

	nu.RawQuery = values.Encode()
	return &nu
}

func urlDelParam(u *url.URL, key string) *url.URL {
	nu := *u
	values := nu.Query()

	values.Del(key)

	nu.RawQuery = values.Encode()
	return &nu
}

func toInt64(i any) (int64, error) {
	switch v := i.(type) {
	case int:
		return int64(v), nil
	case int8:
		return int64(v), nil
	case int16:
		return int64(v), nil
	case int32:
		return int64(v), nil
	case int64:
		return v, nil
	case uint:
		return int64(v), nil
	case uint8:
		return int64(v), nil
	case uint16:
		return int64(v), nil
	case uint32:
		return int64(v), nil

	case string:
		return strconv.ParseInt(v, 10, 64)
	}

	return 0, fmt.Errorf("unable to convert type %T to int", i)
}
