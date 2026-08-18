package main

// Offsite backups to Cloudflare R2.
//
// A backup sitting on the same disk as the data it protects is not a backup —
// one dead disk takes both. R2 is the cheap fix: S3-compatible, no egress
// charge to pull an archive back, and reachable from anywhere.
//
// This talks to R2 directly with SigV4 rather than pulling in an AWS SDK. The
// whole surface needed is PUT, LIST and DELETE on one bucket, which is about a
// hundred lines of signing — far less weight than a dependency that brings its
// own credential chain, region resolution and retry policy.
//
// Uploads use UNSIGNED-PAYLOAD: the archive is hundreds of megabytes, and
// hashing it a second time just to sign it buys nothing over HTTPS.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

type R2Config struct {
	AccountID string
	Bucket    string
	AccessKey string
	SecretKey string
	Prefix    string
	// Endpoint overrides the R2 URL. R2 is S3-compatible, so pointing this at
	// MinIO, Backblaze B2 or S3 itself works — and it is what the tests sign
	// against.
	Endpoint string
}

// r2FromEnv reads the configuration, or reports that offsite backup is off.
func r2FromEnv() (R2Config, bool) {
	c := R2Config{
		AccountID: strings.TrimSpace(os.Getenv("GITGIT_R2_ACCOUNT_ID")),
		Bucket:    strings.TrimSpace(os.Getenv("GITGIT_R2_BUCKET")),
		AccessKey: strings.TrimSpace(os.Getenv("GITGIT_R2_ACCESS_KEY_ID")),
		SecretKey: strings.TrimSpace(os.Getenv("GITGIT_R2_SECRET_ACCESS_KEY")),
		Prefix:    strings.Trim(strings.TrimSpace(os.Getenv("GITGIT_R2_PREFIX")), "/"),
		Endpoint:  strings.TrimRight(strings.TrimSpace(os.Getenv("GITGIT_R2_ENDPOINT")), "/"),
	}
	if c.Bucket == "" || c.AccessKey == "" || c.SecretKey == "" || (c.AccountID == "" && c.Endpoint == "") {
		return c, false
	}
	return c, true
}

func (c R2Config) endpoint() string {
	if c.Endpoint != "" {
		return c.Endpoint
	}
	return "https://" + c.AccountID + ".r2.cloudflarestorage.com"
}

func (c R2Config) key(name string) string {
	if c.Prefix == "" {
		return name
	}
	return c.Prefix + "/" + name
}

// ---------- SigV4 ----------

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

// sign adds the SigV4 Authorization header. R2 ignores the region but the
// signature must still commit to one, and "auto" is what Cloudflare documents.
func (c R2Config) sign(req *http.Request, payloadHash string, now time.Time) {
	const region, service = "auto", "s3"
	amzDate := now.UTC().Format("20060102T150405Z")
	dateStamp := now.UTC().Format("20060102")

	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", payloadHash)
	if req.Host == "" {
		req.Host = req.URL.Host
	}

	// canonical headers: host plus every x-amz-*, lowercased and sorted
	headers := map[string]string{"host": req.URL.Host}
	for k, v := range req.Header {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "x-amz-") {
			headers[lk] = strings.TrimSpace(v[0])
		}
	}
	names := make([]string, 0, len(headers))
	for k := range headers {
		names = append(names, k)
	}
	sort.Strings(names)
	var canonHeaders strings.Builder
	for _, n := range names {
		canonHeaders.WriteString(n + ":" + headers[n] + "\n")
	}
	signedHeaders := strings.Join(names, ";")

	canonicalRequest := strings.Join([]string{
		req.Method,
		escapePath(req.URL.Path),
		canonicalQuery(req.URL.Query()),
		canonHeaders.String(),
		signedHeaders,
		payloadHash,
	}, "\n")

	scope := strings.Join([]string{dateStamp, region, service, "aws4_request"}, "/")
	hashed := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256", amzDate, scope, hex.EncodeToString(hashed[:]),
	}, "\n")

	k := hmacSHA256([]byte("AWS4"+c.SecretKey), dateStamp)
	k = hmacSHA256(k, region)
	k = hmacSHA256(k, service)
	k = hmacSHA256(k, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(k, stringToSign))

	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		c.AccessKey, scope, signedHeaders, signature))
}

// escapePath encodes each segment the way SigV4 expects: RFC 3986, with the
// separators left alone.
func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
	}
	return strings.Join(parts, "/")
}

func canonicalQuery(v url.Values) string {
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []string
	for _, k := range keys {
		vals := append([]string{}, v[k]...)
		sort.Strings(vals)
		for _, val := range vals {
			out = append(out, url.QueryEscape(k)+"="+url.QueryEscape(val))
		}
	}
	return strings.Join(out, "&")
}

// ---------- operations ----------

var r2Client = &http.Client{Timeout: 30 * time.Minute} // an archive can be large

func (c R2Config) do(req *http.Request, payloadHash string) (*http.Response, error) {
	c.sign(req, payloadHash, time.Now())
	resp, err := r2Client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 600))
		resp.Body.Close()
		return nil, fmt.Errorf("R2 %s %s: %s", req.Method, resp.Status, strings.TrimSpace(string(body)))
	}
	return resp, nil
}

// uploadBackup streams a local archive to R2.
func (c R2Config) uploadBackup(path, name string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	u := c.endpoint() + "/" + c.Bucket + "/" + c.key(name)
	req, err := http.NewRequest(http.MethodPut, u, f)
	if err != nil {
		return err
	}
	req.ContentLength = st.Size()
	req.Header.Set("Content-Type", "application/gzip")
	resp, err := c.do(req, "UNSIGNED-PAYLOAD")
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

type r2Object struct {
	Key          string `xml:"Key"`
	Size         int64  `xml:"Size"`
	LastModified string `xml:"LastModified"`
}

// listBackups returns the archives already offsite, newest first.
func (c R2Config) listBackups() ([]r2Object, error) {
	u := c.endpoint() + "/" + c.Bucket + "/?list-type=2"
	if c.Prefix != "" {
		u += "&prefix=" + url.QueryEscape(c.Prefix+"/")
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	empty := sha256.Sum256(nil)
	resp, err := c.do(req, hex.EncodeToString(empty[:]))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var parsed struct {
		Contents []r2Object `xml:"Contents"`
	}
	if err := xml.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	out := parsed.Contents
	sort.Slice(out, func(i, j int) bool { return out[i].Key > out[j].Key })
	return out, nil
}

func (c R2Config) deleteBackup(key string) error {
	req, err := http.NewRequest(http.MethodDelete, c.endpoint()+"/"+c.Bucket+"/"+key, nil)
	if err != nil {
		return err
	}
	empty := sha256.Sum256(nil)
	resp, err := c.do(req, hex.EncodeToString(empty[:]))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// pruneR2Backups keeps the newest `keep` archives offsite. Retention is
// applied separately from the local copies: the whole point of offsite is that
// it outlives the machine, so it usually wants a longer tail.
func (c R2Config) pruneBackups(keep int) {
	if keep <= 0 {
		return
	}
	objects, err := c.listBackups()
	if err != nil {
		log.Printf("backup: listing R2 for pruning: %v", err)
		return
	}
	for i := keep; i < len(objects); i++ {
		if err := c.deleteBackup(objects[i].Key); err != nil {
			log.Printf("backup: pruning %s from R2: %v", objects[i].Key, err)
			continue
		}
		log.Printf("backup: pruned %s from R2", objects[i].Key)
	}
}

// offsiteBackup ships an archive to R2 when it is configured. Failure is
// logged and reported, never silent: a backup you believe is offsite and is
// not is worse than no offsite backup at all.
func offsiteBackup(path, name string) error {
	cfg, ok := r2FromEnv()
	if !ok {
		return nil
	}
	start := time.Now()
	if err := cfg.uploadBackup(path, name); err != nil {
		log.Printf("backup: R2 upload of %s FAILED: %v", name, err)
		return err
	}
	log.Printf("backup: %s uploaded to R2 bucket %q in %s", name, cfg.Bucket, time.Since(start).Round(time.Second))
	cfg.pruneBackups(r2Keep)
	return nil
}
