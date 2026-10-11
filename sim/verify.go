// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package sim

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/url"
)

// The account's verification steps, as open-source tools describe them:
//
//   - /pass/getCode gives a captcha image and sets the "ick" cookie;
//   - /identity/authStart (the notificationUrl) opens the check,
//     /identity/list says which ways it offers (e-mail and SMS);
//   - /identity/auth/sendEmailTicket and sendPhoneTicket "send" the code
//     (CodesSent records them);
//   - /identity/auth/verifyEmail (_flag 8) and verifyPhone (_flag 4) take
//     the code as "ticket" and answer the address of
//     /identity/result/check, which redirects to
//     /pass/serviceLoginAuth2/end, which gives ssecurity in its
//     extension-pragma header and redirects to the sts address.
//
// Every step checks the check's context; the code must be sent by the way
// it is verified with. The caller holds c.mu.

func (c *Cloud) captchaText() string {
	if c.CaptchaText == "" {
		return "demo7"
	}
	return c.CaptchaText
}

func (c *Cloud) code() string {
	if c.Code == "" {
		return "246810"
	}
	return c.Code
}

// CodesSent lists the ways codes were sent for the pending check.
func (c *Cloud) CodesSent() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.sent...)
}

// Verified counts completed identity checks.
func (c *Cloud) Verified() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.verified
}

// captchaPNG is a small image standing for the captcha.
func captchaPNG() []byte {
	img := image.NewGray(image.Rect(0, 0, 8, 4))
	for x := 0; x < 8; x += 2 {
		img.SetGray(x, 1, color.Gray{Y: 255})
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func (c *Cloud) verifyStep(w http.ResponseWriter, r *http.Request, start func(any)) {
	q := r.URL.Query()
	_ = r.ParseForm()
	way := map[string]string{
		"/identity/auth/sendEmailTicket": "email", "/identity/auth/sendPhoneTicket": "phone",
		"/identity/auth/verifyEmail": "email", "/identity/auth/verifyPhone": "phone",
	}[r.URL.Path]
	if r.URL.Path == "/pass/getCode" {
		c.ick = base64.RawURLEncoding.EncodeToString(rnd(12))
		http.SetCookie(w, &http.Cookie{Name: "ick", Value: c.ick, Path: "/"})
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(captchaPNG())
		return
	}
	if c.vctx == "" || q.Get("context") != c.vctx {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	switch r.URL.Path {
	case "/identity/authStart":
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>verify</body></html>"))
	case "/identity/list":
		start(map[string]any{"code": 0, "options": []int{4, 8}, "flag": 8})
	case "/identity/auth/sendEmailTicket", "/identity/auth/sendPhoneTicket":
		if r.Method != http.MethodPost || q.Get("sid") != "xiaomiio" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		c.sent = append(c.sent, way)
		start(map[string]any{"code": 0})
	case "/identity/auth/verifyEmail", "/identity/auth/verifyPhone":
		flag := map[string]string{"email": "8", "phone": "4"}[way]
		if r.Method != http.MethodPost || r.PostForm.Get("_flag") != flag || q.Get("_flag") != flag {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		wasSent := false
		for _, s := range c.sent {
			wasSent = wasSent || s == way
		}
		if !wasSent || r.PostForm.Get("ticket") != c.code() {
			start(map[string]any{"code": 70014, "tips": "wrong code"})
			return
		}
		next := url.Values{"sid": {"xiaomiio"}, "context": {c.vctx}, "_t": {"1"}}
		start(map[string]any{"code": 0, "location": c.HTTP.URL + "/identity/result/check?" + next.Encode()})
	case "/identity/result/check":
		next := url.Values{"context": {c.vctx}, "_t": {"1"}}
		w.Header().Set("Location", c.HTTP.URL+"/pass/serviceLoginAuth2/end?"+next.Encode())
		w.WriteHeader(http.StatusFound)
	case "/pass/serviceLoginAuth2/end":
		c.ssecurity = base64.StdEncoding.EncodeToString(rnd(16))
		prag, _ := json.Marshal(map[string]any{"ssecurity": c.ssecurity, "psecurity": "ps", "nonce": 1})
		w.Header().Set("extension-pragma", string(prag))
		http.SetCookie(w, &http.Cookie{Name: "userId", Value: "1234567", Path: "/"})
		w.Header().Set("Location", c.HTTP.URL+"/sts?d=1&ticket=0")
		w.WriteHeader(http.StatusFound)
		c.vctx, c.sent = "", nil
		c.verified++
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}
