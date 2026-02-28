package main

import (
	"fmt"
	"net/http"
	"strconv"
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
