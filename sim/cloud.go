package sim

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	"github.com/cB-Sigma/miio"
)

// Cloud simulates the Mi account as open-source tools describe it: the
// three sign-in steps (serviceLogin answering a _sign after
// "&&&START&&&"; serviceLoginAuth2 checking the user, the upper-case MD5
// of the password and the _sign, and answering ssecurity, userId and a
// location; the location setting the serviceToken cookie) and the
// RC4-encrypted API under /<region>/app/v2/…, which checks the
// serviceToken cookie and both signatures and answers encrypted JSON.
type Cloud struct {
	HTTP           *httptest.Server
	User, Password string
	// Verify asks for an identity check (notificationUrl) at every
	// sign-in; it is completed by asking for a code by e-mail or SMS and
	// sending back Code (verify.go).
	Verify bool
	// Captcha asks for a captcha at sign-in until CaptchaText is sent
	// with the "ick" cookie the captcha image set.
	Captcha bool
	// Code and CaptchaText are what the simulated person receives and
	// reads ("246810" and "demo7" when empty).
	Code, CaptchaText string
	// Homes are the account's devices by region.
	Devices map[string][]miio.CloudDevice

	mu        sync.Mutex
	ssecurity string
	token     string
	logins    int
	calls     []string
	vctx      string   // the pending identity check's context
	sent      []string // ways codes were sent ("email", "phone")
	ick       string
	verified  int
	linked    map[string]*Device // devices reached through the account, by did
}

// StartCloud starts the simulated account.
func StartCloud(user, password string, devices map[string][]miio.CloudDevice) *Cloud {
	c := &Cloud{User: user, Password: password, Devices: devices}
	c.HTTP = httptest.NewServer(http.HandlerFunc(c.serve))
	return c
}

// Close stops it.
func (c *Cloud) Close() { c.HTTP.Close() }

// APIBase is the region's API address, for miio.Cloud's APIBase.
func (c *Cloud) APIBase(region string) string { return c.HTTP.URL + "/" + region + "/app" }

// Logins counts completed sign-ins.
func (c *Cloud) Logins() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.logins
}

// EndSession forgets the service token (the session expired).
func (c *Cloud) EndSession() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = ""
}

func rnd(n int) []byte { b := make([]byte, n); _, _ = rand.Read(b); return b }

func (c *Cloud) serve(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	start := func(v any) {
		b, _ := json.Marshal(v)
		_, _ = w.Write(append([]byte("&&&START&&&"), b...))
	}
	_ = r.ParseForm()
	q := r.Form // the query and a form body alike
	switch {
	case r.URL.Path == "/pass/serviceLogin":
		if q.Get("sid") != "xiaomiio" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		start(map[string]any{"_sign": "SIMSIGN+abc=", "code": 70016, "desc": "login required"})
	case r.URL.Path == "/pass/serviceLoginAuth2" && r.Method == http.MethodPost:
		sum := md5.Sum([]byte(c.Password))
		if q.Get("_sign") != "SIMSIGN+abc=" || q.Get("sid") != "xiaomiio" || q.Get("callback") == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if q.Get("user") != c.User || q.Get("hash") != strings.ToUpper(hex.EncodeToString(sum[:])) {
			start(map[string]any{"code": 70016, "result": "error", "desc": "wrong password"})
			return
		}
		if c.Captcha {
			ick, _ := r.Cookie("ick")
			given := q.Get("captCode")
			if given == "" || ick == nil || ick.Value != c.ick || !strings.EqualFold(given, c.captchaText()) {
				code := 87001
				if given == "" {
					code = 70016
				}
				start(map[string]any{"code": code, "captchaUrl": "/pass/getCode?icodeType=login"})
				return
			}
		}
		if c.Verify {
			c.vctx = base64.RawURLEncoding.EncodeToString(rnd(9))
			c.sent = nil
			start(map[string]any{"code": 0, "notificationUrl": c.HTTP.URL + "/identity/authStart?sid=xiaomiio&context=" + c.vctx})
			return
		}
		c.ssecurity = base64.StdEncoding.EncodeToString(rnd(16))
		start(map[string]any{"code": 0, "result": "ok", "ssecurity": c.ssecurity, "userId": 1234567, "cUserId": "cu", "passToken": "pt",
			"location": c.HTTP.URL + "/sts?d=1&ticket=0"})
	case r.URL.Path == "/sts":
		c.token = base64.StdEncoding.EncodeToString(rnd(24))
		c.logins++
		http.SetCookie(w, &http.Cookie{Name: "serviceToken", Value: c.token, Path: "/"})
		_, _ = w.Write([]byte("ok"))
	case strings.Contains(r.URL.Path, "/app/") && r.Method == http.MethodPost:
		c.api(w, r)
	case strings.HasPrefix(r.URL.Path, "/identity/") || r.URL.Path == "/pass/getCode" || r.URL.Path == "/pass/serviceLoginAuth2/end":
		c.verifyStep(w, r, start)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (c *Cloud) api(w http.ResponseWriter, r *http.Request) {
	ck, _ := r.Cookie("serviceToken")
	if ck == nil || c.token == "" || ck.Value != c.token {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	_ = r.ParseForm()
	q := r.Form // the query and a form body alike
	i := strings.Index(r.URL.Path, "/app/")
	region := strings.Trim(r.URL.Path[:i], "/")
	path := r.URL.Path[i+len("/app"):]
	sn := miio.SignedNonce(c.ssecurity, q.Get("_nonce"))
	encData, encHash := q.Get("data"), q.Get("rc4_hash__")
	if q.Get("signature") != miio.EncSignature("POST", path, [][2]string{{"data", encData}, {"rc4_hash__", encHash}}, sn) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	data, err1 := miio.RC4Decrypt(sn, encData)
	hash, err2 := miio.RC4Decrypt(sn, encHash)
	if err1 != nil || err2 != nil || string(hash) != miio.EncSignature("POST", path, [][2]string{{"data", string(data)}}, sn) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	c.calls = append(c.calls, region+path)
	var in map[string]any
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	_ = dec.Decode(&in)
	var res any
	devs := c.Devices[region]
	switch path {
	case "/v2/homeroom/gethome":
		var hl []any
		if len(devs) > 0 {
			hl = append(hl, map[string]any{"id": 100001, "name": "Home", "uid": 1234567})
		}
		res = map[string]any{"homelist": hl, "has_more": false}
	case "/v2/user/get_device_cnt":
		res = map[string]any{"share": map[string]any{"share_family": []any{}}, "own": map[string]any{"device_cnt": len(devs)}}
	case "/v2/home/home_device_list":
		if fmt.Sprint(in["home_id"]) != "100001" || fmt.Sprint(in["home_owner"]) != "1234567" {
			res = map[string]any{"device_info": nil}
			break
		}
		res = map[string]any{"device_info": devs, "has_more": false}
	case "/home/device_list":
		// The older list some clients (micloud) use.
		res = map[string]any{"list": devs}
	default:
		res = c.relay(path, data)
	}
	out := map[string]any{"code": 0, "message": "ok", "result": res}
	if res == nil {
		out = map[string]any{"code": -1, "message": "unknown path"}
	}
	b, _ := json.Marshal(out)
	_, _ = w.Write([]byte(miio.RC4Encrypt(sn, string(b))))
}

// SpecServer serves MIoT specifications as miot-spec.org does:
// /instances?status=released lists models with their type URNs, and
// /instance?type=<URN> gives one.
type SpecServer struct {
	HTTP  *httptest.Server
	mu    sync.Mutex
	specs map[string]miio.Spec
	hits  int
}

// StartSpecServer serves the specifications by model.
func StartSpecServer(specs map[string]miio.Spec) *SpecServer {
	s := &SpecServer{specs: specs}
	s.HTTP = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.hits++
		switch r.URL.Path {
		case "/miot-spec-v2/instances":
			var ins []any
			for m, sp := range s.specs {
				ins = append(ins, map[string]any{"model": m, "version": 1, "type": sp.Type, "ts": 1600000000})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"instances": ins})
		case "/miot-spec-v2/instance":
			for _, sp := range s.specs {
				if sp.Type == r.URL.Query().Get("type") {
					_ = json.NewEncoder(w).Encode(sp)
					return
				}
			}
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	return s
}

// URL is the specification service's base (…/miot-spec-v2).
func (s *SpecServer) URL() string { return s.HTTP.URL + "/miot-spec-v2" }

// Hits counts requests.
func (s *SpecServer) Hits() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits
}

// Close stops it.
func (s *SpecServer) Close() { s.HTTP.Close() }
