package cookies

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

func TestWriteAndRead(t *testing.T) {
	t.Run("Round trip encodes and decodes cookie value", func(t *testing.T) {
		w := httptest.NewRecorder()

		cookie := http.Cookie{
			Name:  "test_cookie",
			Value: "this is a test value with special chars!便#ي%",
		}

		err := Write(w, cookie)
		assert.Nil(t, err)

		cookies := w.Result().Cookies()
		req := httptest.NewRequest("GET", "/", nil)
		req.AddCookie(cookies[0])

		value, err := Read(req, "test_cookie")
		assert.Nil(t, err)
		assert.Equal(t, "this is a test value with special chars!便#ي%", value)
	})

	t.Run("Returns ErrValueTooLong when cookie exceeds 4096 bytes", func(t *testing.T) {
		w := httptest.NewRecorder()

		cookie := http.Cookie{
			Name:  "test_cookie",
			Value: strings.Repeat("a", 4000),
		}

		err := Write(w, cookie)
		assert.Equal(t, err, ErrValueTooLong)
	})

	t.Run("Returns ErrInvalidValue for tampered base64", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.AddCookie(&http.Cookie{Name: "test", Value: "invalid-base64!"})

		_, err := Read(req, "test")
		assert.Equal(t, err, ErrInvalidValue)
	})
}

func TestWriteSignedAndReadSigned(t *testing.T) {
	t.Run("Round trip signs and verifies cookie value", func(t *testing.T) {
		w := httptest.NewRecorder()

		cookie := http.Cookie{
			Name:  "test_cookie",
			Value: "this is a test value",
		}

		secretKey := "mySecretKeyAX7v2WqLpJ3nZcRYKtM9o"
		err := WriteSigned(w, cookie, secretKey)
		assert.Nil(t, err)

		cookies := w.Result().Cookies()
		req := httptest.NewRequest("GET", "/", nil)
		req.AddCookie(cookies[0])

		t.Log(cookies[0].Value)

		value, err := ReadSigned(req, "test_cookie", secretKey)
		assert.Nil(t, err)
		assert.Equal(t, "this is a test value", value)
	})

	t.Run("Returns ErrInvalidValue with wrong secret key", func(t *testing.T) {
		w := httptest.NewRecorder()
		cookie := http.Cookie{
			Name:  "test_cookie",
			Value: "this is a test value",
		}

		err := WriteSigned(w, cookie, "mySecretKeyAX7v2WqLpJ3nZcRYKtM9o")
		assert.Nil(t, err)

		cookies := w.Result().Cookies()
		req := httptest.NewRequest("GET", "/", nil)
		req.AddCookie(cookies[0])

		_, err = ReadSigned(req, "test_cookie", "wrongSecretKeyAX7v2WqLpJ3nZcRYKt")
		assert.Equal(t, err, ErrInvalidValue)
	})

	t.Run("Returns ErrInvalidValue for tampered signature", func(t *testing.T) {
		w := httptest.NewRecorder()
		cookie := http.Cookie{
			Name:  "test_cookie",
			Value: "this is a test value",
		}

		secretKey := "mySecretKeyAX7v2WqLpJ3nZcRYKtM9o"
		err := WriteSigned(w, cookie, secretKey)
		assert.Nil(t, err)

		cookies := w.Result().Cookies()

		if cookies[0].Value[0] == 'a' {
			cookies[0].Value = "b" + cookies[0].Value[1:]
		} else {
			cookies[0].Value = "a" + cookies[0].Value[1:]
		}

		req := httptest.NewRequest("GET", "/", nil)
		req.AddCookie(cookies[0])

		_, err = ReadSigned(req, "test_cookie", secretKey)
		assert.Equal(t, err, ErrInvalidValue)
	})

	t.Run("Returns ErrInvalidValue for value shorter than signature", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)

		req.AddCookie(&http.Cookie{Name: "test_cookie", Value: "dGVzdA=="})

		_, err := ReadSigned(req, "test_cookie", "mySecretKeyAX7v2WqLpJ3nZcRYKtM9o")
		assert.Equal(t, err, ErrInvalidValue)
	})
}

func TestWriteEncryptedAndReadEncrypted(t *testing.T) {
	t.Run("Round trip encrypts and decrypts cookie value", func(t *testing.T) {
		w := httptest.NewRecorder()
		cookie := http.Cookie{
			Name:  "test_cookie",
			Value: "this is a test value",
		}

		secretKey := "mySecretKeyAX7v2WqLpJ3nZcRYKtM9o"
		err := WriteEncrypted(w, cookie, secretKey)
		assert.Nil(t, err)

		cookies := w.Result().Cookies()

		decodedValue, err := base64.URLEncoding.DecodeString(cookies[0].Value)
		assert.Nil(t, err)
		assert.False(t, strings.Contains(string(decodedValue), "this is a test value"))

		req := httptest.NewRequest("GET", "/", nil)
		req.AddCookie(cookies[0])

		value, err := ReadEncrypted(req, "test_cookie", secretKey)
		assert.Nil(t, err)
		assert.Equal(t, "this is a test value", value)
	})

	t.Run("Returns ErrInvalidValue with wrong decryption key", func(t *testing.T) {
		w := httptest.NewRecorder()
		cookie := http.Cookie{
			Name:  "test_cookie",
			Value: "this is a test value",
		}

		secretKey := "mySecretKeyAX7v2WqLpJ3nZcRYKtM9o"
		err := WriteEncrypted(w, cookie, secretKey)
		assert.Nil(t, err)

		cookies := w.Result().Cookies()
		req := httptest.NewRequest("GET", "/", nil)
		req.AddCookie(cookies[0])

		_, err = ReadEncrypted(req, "test_cookie", "wrongSecretKeyAX7v2WqLpJ3nZcRYKt")
		assert.Equal(t, err, ErrInvalidValue)
	})

	t.Run("Returns ErrInvalidValue for tampered encrypted data", func(t *testing.T) {
		w := httptest.NewRecorder()
		cookie := http.Cookie{
			Name:  "test_cookie",
			Value: "this is a test value",
		}

		secretKey := "mySecretKeyAX7v2WqLpJ3nZcRYKtM9o"
		err := WriteEncrypted(w, cookie, secretKey)
		assert.Nil(t, err)

		cookies := w.Result().Cookies()

		if cookies[0].Value[0] == 'a' {
			cookies[0].Value = "b" + cookies[0].Value[1:]
		} else {
			cookies[0].Value = "a" + cookies[0].Value[1:]
		}

		req := httptest.NewRequest("GET", "/", nil)
		req.AddCookie(cookies[0])

		_, err = ReadEncrypted(req, "test_cookie", secretKey)
		assert.Equal(t, err, ErrInvalidValue)
	})

	t.Run("Returns ErrInvalidValue for value shorter than nonce", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)

		req.AddCookie(&http.Cookie{Name: "test", Value: "dGVzdA=="})

		_, err := ReadEncrypted(req, "test", "mySecretKeyAX7v2WqLpJ3nZcRYKtM9o")
		assert.Equal(t, err, ErrInvalidValue)
	})
}
