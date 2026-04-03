package main

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pascaldekloe/jwt"
)

func (app *application) newEmailData() map[string]any {
	data := map[string]any{
		"BaseURL": app.config.baseURL,
	}

	return data
}

func (app *application) newAuthenticationToken(userID int64) (string, time.Time, error) {
	now := time.Now()

	var claims jwt.Claims
	claims.Subject = strconv.FormatInt(userID, 10)

	expiry := now.Add(24 * time.Hour)
	claims.Issued = jwt.NewNumericTime(now)
	claims.NotBefore = jwt.NewNumericTime(now)
	claims.Expires = jwt.NewNumericTime(expiry)

	claims.Issuer = app.config.baseURL
	claims.Audiences = []string{app.config.baseURL}

	jwt, err := claims.HMACSign(jwt.HS256, []byte(app.config.jwt.secretKey))
	return string(jwt), expiry, err
}

func (app *application) backgroundTask(r *http.Request, fn func() error) {
	app.wg.Add(1)

	go func() {
		defer app.wg.Done()

		defer func() {
			pv := recover()
			if pv != nil {
				app.reportServerError(r, fmt.Errorf("%v", pv))
			}
		}()

		err := fn()
		if err != nil {
			app.reportServerError(r, err)
		}
	}()
}

type listQuery struct {
	Page     int
	PageSize int
	Sort     string
	Order    string
	Search   string
}

func readListQuery(values url.Values, allowedSorts map[string]string) (listQuery, map[string]string) {
	q := listQuery{
		Page:     1,
		PageSize: 25,
		Sort:     defaultListSort(allowedSorts),
		Order:    "desc",
		Search:   strings.TrimSpace(values.Get("q")),
	}

	errs := map[string]string{}

	if raw := strings.TrimSpace(values.Get("page")); raw != "" {
		page, err := strconv.Atoi(raw)
		if err != nil || page < 1 {
			errs["page"] = "must be a positive integer"
		} else {
			q.Page = page
		}
	}

	if raw := strings.TrimSpace(values.Get("page_size")); raw != "" {
		pageSize, err := strconv.Atoi(raw)
		if err != nil || pageSize < 1 || pageSize > 100 {
			errs["page_size"] = "must be between 1 and 100"
		} else {
			q.PageSize = pageSize
		}
	}

	if raw := strings.TrimSpace(values.Get("sort")); raw != "" {
		if column, ok := allowedSorts[raw]; ok {
			q.Sort = column
		} else {
			errs["sort"] = "must be one of the supported sort fields"
		}
	}

	if raw := strings.TrimSpace(values.Get("order")); raw != "" {
		raw = strings.ToLower(raw)
		if raw != "asc" && raw != "desc" {
			errs["order"] = "must be asc or desc"
		} else {
			q.Order = raw
		}
	}

	if len(errs) == 0 {
		return q, nil
	}
	return q, errs
}

func defaultListSort(allowedSorts map[string]string) string {
	if len(allowedSorts) == 0 {
		return "created_at"
	}
	if column, ok := allowedSorts["created_at"]; ok {
		return column
	}
	if column, ok := allowedSorts["name"]; ok {
		return column
	}

	keys := make([]string, 0, len(allowedSorts))
	for key := range allowedSorts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return allowedSorts[keys[0]]
}
