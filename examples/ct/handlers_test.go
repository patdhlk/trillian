package ct

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/golang/glog"
	"github.com/golang/mock/gomock"
	ct "github.com/google/certificate-transparency/go"
	"github.com/google/certificate-transparency/go/tls"
	"github.com/google/certificate-transparency/go/x509"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/examples/ct/testonly"
	"github.com/google/trillian/mockclient"
	"github.com/google/trillian/util"
	"golang.org/x/net/context"
)

// Arbitrary time for use in tests
var fakeTime = time.Date(2016, 7, 22, 11, 01, 13, 0, time.UTC)

// The deadline should be the above bumped by 500ms
var fakeDeadlineTime = time.Date(2016, 7, 22, 11, 01, 13, 500*1000*1000, time.UTC)
var fakeTimeSource = util.FakeTimeSource{FakeTime: fakeTime}
var okStatus = &trillian.TrillianApiStatus{StatusCode: trillian.TrillianApiStatusCode_OK}

const caCertB64 string = `MIIC0DCCAjmgAwIBAgIBADANBgkqhkiG9w0BAQUFADBVMQswCQYDVQQGEwJHQjEk
MCIGA1UEChMbQ2VydGlmaWNhdGUgVHJhbnNwYXJlbmN5IENBMQ4wDAYDVQQIEwVX
YWxlczEQMA4GA1UEBxMHRXJ3IFdlbjAeFw0xMjA2MDEwMDAwMDBaFw0yMjA2MDEw
MDAwMDBaMFUxCzAJBgNVBAYTAkdCMSQwIgYDVQQKExtDZXJ0aWZpY2F0ZSBUcmFu
c3BhcmVuY3kgQ0ExDjAMBgNVBAgTBVdhbGVzMRAwDgYDVQQHEwdFcncgV2VuMIGf
MA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDVimhTYhCicRmTbneDIRgcKkATxtB7
jHbrkVfT0PtLO1FuzsvRyY2RxS90P6tjXVUJnNE6uvMa5UFEJFGnTHgW8iQ8+EjP
KDHM5nugSlojgZ88ujfmJNnDvbKZuDnd/iYx0ss6hPx7srXFL8/BT/9Ab1zURmnL
svfP34b7arnRsQIDAQABo4GvMIGsMB0GA1UdDgQWBBRfnYgNyHPmVNT4DdjmsMEk
tEfDVTB9BgNVHSMEdjB0gBRfnYgNyHPmVNT4DdjmsMEktEfDVaFZpFcwVTELMAkG
A1UEBhMCR0IxJDAiBgNVBAoTG0NlcnRpZmljYXRlIFRyYW5zcGFyZW5jeSBDQTEO
MAwGA1UECBMFV2FsZXMxEDAOBgNVBAcTB0VydyBXZW6CAQAwDAYDVR0TBAUwAwEB
/zANBgkqhkiG9w0BAQUFAAOBgQAGCMxKbWTyIF4UbASydvkrDvqUpdryOvw4BmBt
OZDQoeojPUApV2lGOwRmYef6HReZFSCa6i4Kd1F2QRIn18ADB8dHDmFYT9czQiRy
f1HWkLxHqd81TbD26yWVXeGJPE3VICskovPkQNJ0tU4b03YmnKliibduyqQQkOFP
OwqULg==`

const intermediateCertB64 string = `MIIC3TCCAkagAwIBAgIBCTANBgkqhkiG9w0BAQUFADBVMQswCQYDVQQGEwJHQjEk
MCIGA1UEChMbQ2VydGlmaWNhdGUgVHJhbnNwYXJlbmN5IENBMQ4wDAYDVQQIEwVX
YWxlczEQMA4GA1UEBxMHRXJ3IFdlbjAeFw0xMjA2MDEwMDAwMDBaFw0yMjA2MDEw
MDAwMDBaMGIxCzAJBgNVBAYTAkdCMTEwLwYDVQQKEyhDZXJ0aWZpY2F0ZSBUcmFu
c3BhcmVuY3kgSW50ZXJtZWRpYXRlIENBMQ4wDAYDVQQIEwVXYWxlczEQMA4GA1UE
BxMHRXJ3IFdlbjCBnzANBgkqhkiG9w0BAQEFAAOBjQAwgYkCgYEA12pnjRFvUi5V
/4IckGQlCLcHSxTXcRWQZPeSfv3tuHE1oTZe594Yy9XOhl+GDHj0M7TQ09NAdwLn
o+9UKx3+m7qnzflNxZdfxyn4bxBfOBskNTXPnIAPXKeAwdPIRADuZdFu6c9S24rf
/lD1xJM1CyGQv1DVvDbzysWo2q6SzYsCAwEAAaOBrzCBrDAdBgNVHQ4EFgQUllUI
BQJ4R56Hc3ZBMbwUOkfiKaswfQYDVR0jBHYwdIAUX52IDchz5lTU+A3Y5rDBJLRH
w1WhWaRXMFUxCzAJBgNVBAYTAkdCMSQwIgYDVQQKExtDZXJ0aWZpY2F0ZSBUcmFu
c3BhcmVuY3kgQ0ExDjAMBgNVBAgTBVdhbGVzMRAwDgYDVQQHEwdFcncgV2VuggEA
MAwGA1UdEwQFMAMBAf8wDQYJKoZIhvcNAQEFBQADgYEAIgbascZrcdzglcP2qi73
LPd2G+er1/w5wxpM/hvZbWc0yoLyLd5aDIu73YJde28+dhKtjbMAp+IRaYhgIyYi
hMOqXSGR79oQv5I103s6KjQNWUGblKSFZvP6w82LU9Wk6YJw6tKXsHIQ+c5KITix
iBEUO5P6TnqH3TfhOF8sKQg=`

const caAndIntermediateCertsPEM string = "-----BEGIN CERTIFICATE-----\n" + caCertB64 + "\n-----END CERTIFICATE-----\n" +
	"\n-----BEGIN CERTIFICATE-----\n" + intermediateCertB64 + "\n-----END CERTIFICATE-----\n"

type handlerTestInfo struct {
	mockCtrl *gomock.Controller
	km       *crypto.MockKeyManager
	roots    *PEMCertPool
	client   *mockclient.MockTrillianLogClient
	c        LogContext
}

// setupTest creates mock objects and contexts.  Caller should invoke info.mockCtrl.Finish().
func setupTest(t *testing.T, pemRoots []string) handlerTestInfo {
	info := handlerTestInfo{}
	info.mockCtrl = gomock.NewController(t)
	info.km = crypto.NewMockKeyManager(info.mockCtrl)
	info.km.EXPECT().GetRawPublicKey().AnyTimes().Return([]byte("key"), nil)
	info.km.EXPECT().SignatureAlgorithm().AnyTimes().Return(trillian.SignatureAlgorithm_ECDSA)
	info.client = mockclient.NewMockTrillianLogClient(info.mockCtrl)
	info.roots = NewPEMCertPool()
	for _, pemRoot := range pemRoots {
		if !info.roots.AppendCertsFromPEM([]byte(pemRoot)) {
			glog.Fatal("failed to load cert pool")
		}
	}
	info.c = *NewLogContext(0x42, "test", info.roots, info.client, info.km, time.Millisecond*500, fakeTimeSource)
	return info
}

func (info handlerTestInfo) expectSign(toSign string) {
	data, _ := hex.DecodeString(toSign)
	mockSigner := crypto.NewMockSigner(info.mockCtrl)
	mockSigner.EXPECT().Sign(gomock.Any(), data, gomock.Any()).AnyTimes().Return([]byte("signed"), nil)
	info.km.EXPECT().Signer().AnyTimes().Return(mockSigner, nil)
}

func (info handlerTestInfo) getHandlers() map[string]appHandler {
	return map[string]appHandler{
		"get-sth":             appHandler{context: info.c, handler: getSTH, name: "GetSTH", method: http.MethodGet},
		"get-sth-consistency": appHandler{context: info.c, handler: getSTHConsistency, name: "GetSTHConsistency", method: http.MethodGet},
		"get-proof-by-hash":   appHandler{context: info.c, handler: getProofByHash, name: "GetProofByHash", method: http.MethodGet},
		"get-entries":         appHandler{context: info.c, handler: getEntries, name: "GetEntries", method: http.MethodGet},
		"get-roots":           appHandler{context: info.c, handler: getRoots, name: "GetRoots", method: http.MethodGet},
		"get-entry-and-proof": appHandler{context: info.c, handler: getEntryAndProof, name: "GetEntryAndProof", method: http.MethodGet},
	}
}

func (info handlerTestInfo) postHandlers() map[string]appHandler {
	return map[string]appHandler{
		"add-chain":     appHandler{context: info.c, handler: addChain, name: "AddChain", method: http.MethodPost},
		"add-pre-chain": appHandler{context: info.c, handler: addPreChain, name: "AddPreChain", method: http.MethodPost},
	}
}

func TestPostHandlersRejectGet(t *testing.T) {
	info := setupTest(t, []string{testonly.FakeCACertPEM})
	defer info.mockCtrl.Finish()

	// Anything in the post handler list should reject GET
	for path, handler := range info.postHandlers() {
		s := httptest.NewServer(handler)
		defer s.Close()

		resp, err := http.Get(s.URL + "/ct/v1/" + path)
		if err != nil {
			t.Errorf("http.Get(%s)=(_,%q); want (_,nil)", path, err)
			continue
		}
		if got, want := resp.StatusCode, http.StatusMethodNotAllowed; got != want {
			t.Errorf("http.Get(%s)=(%d,nil); want (%d,nil)", path, got, want)
		}

	}
}

func TestGetHandlersRejectPost(t *testing.T) {
	info := setupTest(t, []string{testonly.FakeCACertPEM})
	defer info.mockCtrl.Finish()

	// Anything in the get handler list should reject POST.
	for path, handler := range info.getHandlers() {
		s := httptest.NewServer(handler)
		defer s.Close()

		resp, err := http.Post(s.URL+"/ct/v1/"+path, "application/json", nil)
		if err != nil {
			t.Errorf("http.Post(%s)=(_,%q); want (_,nil)", path, err)
			continue
		}
		if got, want := resp.StatusCode, http.StatusMethodNotAllowed; got != want {
			t.Errorf("http.Post(%s)=(%d,nil); want (%d,nil)", path, got, want)
		}
	}
}

func TestPostHandlersFailure(t *testing.T) {
	var tests = []struct {
		descr string
		body  io.Reader
		want  int
	}{
		{"nil", nil, http.StatusBadRequest},
		{"''", strings.NewReader(""), http.StatusBadRequest},
		{"malformed-json", strings.NewReader("{ !$%^& not valid json "), http.StatusBadRequest},
		{"empty-chain", strings.NewReader(`{ "chain": [] }`), http.StatusBadRequest},
		{"wrong-chain", strings.NewReader(`{ "chain": [ "test" ] }`), http.StatusBadRequest},
	}

	info := setupTest(t, []string{testonly.FakeCACertPEM})
	defer info.mockCtrl.Finish()
	for path, handler := range info.postHandlers() {
		s := httptest.NewServer(handler)
		defer s.Close()

		for _, test := range tests {
			resp, err := http.Post(s.URL+"/ct/v1/"+path, "application/json", test.body)
			if err != nil {
				t.Errorf("http.Post(%s,%s)=(_,%q); want (_,nil)", path, test.descr, err)
				continue
			}
			if resp.StatusCode != test.want {
				t.Errorf("http.Post(%s,%s)=(%d,nil); want (%d,nil)", path, test.descr, resp.StatusCode, test.want)
			}
		}
	}
}

func TestGetRoots(t *testing.T) {
	info := setupTest(t, []string{caAndIntermediateCertsPEM})
	defer info.mockCtrl.Finish()
	handler := appHandler{context: info.c, handler: getRoots, name: "GetRoots", method: http.MethodGet}

	req, err := http.NewRequest("GET", "http://example.com/ct/v1/get-roots", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if got, want := w.Code, http.StatusOK; got != want {
		t.Fatalf("http.Get(get-roots)=%d; want %d", got, want)
	}

	var parsedJSON map[string][]string
	if err := json.Unmarshal(w.Body.Bytes(), &parsedJSON); err != nil {
		t.Fatalf("json.Unmarshal(%q)=%q; want nil", w.Body.Bytes(), err)
	}
	if got := len(parsedJSON); got != 1 {
		t.Errorf("len(json)=%d; want 1", got)
	}
	certs := parsedJSON[jsonMapKeyCertificates]
	if got := len(certs); got != 2 {
		t.Fatalf("len(%q)=%d; want 2", certs, got)
	}
	if got, want := certs[0], strings.Replace(caCertB64, "\n", "", -1); got != want {
		t.Errorf("certs[0]=%s; want %s", got, want)
	}
	if got, want := certs[1], strings.Replace(intermediateCertB64, "\n", "", -1); got != want {
		t.Errorf("certs[1]=%s; want %s", got, want)
	}
}

func TestAddChain(t *testing.T) {
	var tests = []struct {
		descr     string
		chain     []string
		toSign    string // hex-encoded
		rpcStatus trillian.TrillianApiStatusCode
		want      int
	}{
		{
			descr: "leaf-only",
			chain: []string{testonly.LeafSignedByFakeIntermediateCertPEM},
			want:  http.StatusBadRequest,
		},
		{
			descr: "wrong-entry-type",
			chain: []string{testonly.PrecertPEMValid},
			want:  http.StatusBadRequest,
		},
		{
			descr:     "backend-rpc-fail",
			chain:     []string{testonly.LeafSignedByFakeIntermediateCertPEM, testonly.FakeIntermediateCertPEM},
			toSign:    "1337d72a403b6539f58896decba416d5d4b3603bfa03e1f94bb9b4e898af897d",
			rpcStatus: trillian.TrillianApiStatusCode_ERROR,
			want:      http.StatusInternalServerError,
		},
		{
			descr:     "success",
			chain:     []string{testonly.LeafSignedByFakeIntermediateCertPEM, testonly.FakeIntermediateCertPEM},
			toSign:    "1337d72a403b6539f58896decba416d5d4b3603bfa03e1f94bb9b4e898af897d",
			rpcStatus: trillian.TrillianApiStatusCode_OK,
			want:      http.StatusOK,
		},
	}
	info := setupTest(t, []string{testonly.FakeCACertPEM})
	defer info.mockCtrl.Finish()

	for _, test := range tests {
		pool := loadCertsIntoPoolOrDie(t, test.chain)
		chain := createJSONChain(t, *pool)
		if len(test.toSign) > 0 {
			info.expectSign(test.toSign)
			merkleLeaf, _, err := signV1SCTForCertificate(info.km, pool.RawCertificates()[0], nil, fakeTime)
			if err != nil {
				t.Errorf("Unexpected error signing SCT: %v", err)
				continue
			}
			leaves := logLeavesForCert(t, info.km, pool.RawCertificates(), merkleLeaf, false)
			info.client.EXPECT().QueueLeaves(deadlineMatcher(), &trillian.QueueLeavesRequest{LogId: 0x42, Leaves: leaves}).Return(&trillian.QueueLeavesResponse{Status: &trillian.TrillianApiStatus{StatusCode: test.rpcStatus}}, nil)
		}

		recorder := makeAddChainRequest(t, info.c, chain)
		if recorder.Code != test.want {
			t.Errorf("addChain(%s)=%d (body:%v); want %dv", test.descr, recorder.Code, recorder.Body, test.want)
			continue
		}
		if test.want == http.StatusOK {
			var resp ct.AddChainResponse
			if err := json.NewDecoder(recorder.Body).Decode(&resp); err != nil {
				t.Fatalf("json.Decode(%s)=%v; want nil", recorder.Body.Bytes(), err)
			}

			if got, want := ct.Version(resp.SCTVersion), ct.V1; got != want {
				t.Errorf("resp.SCTVersion=%v; want %v", got, want)
			}
			if got, want := hex.EncodeToString(resp.ID), ctMockLogID; got != want {
				t.Errorf("resp.ID=%s; want %s", got, want)
			}
			if got, want := resp.Timestamp, uint64(1469185273000); got != want {
				t.Errorf("resp.Timestamp=%d; want %d", got, want)
			}
			if got, want := hex.EncodeToString(resp.Signature), "040300067369676e6564"; got != want {
				t.Errorf("resp.Signature=%s; want %s", got, want)
			}
		}
	}
}

func TestAddPrechain(t *testing.T) {
	var tests = []struct {
		descr     string
		chain     []string
		toSign    string // hex-encoded
		rpcStatus trillian.TrillianApiStatusCode
		want      int
	}{
		{
			descr: "leaf-signed-by-different",
			chain: []string{testonly.PrecertPEMValid, testonly.FakeIntermediateCertPEM},
			want:  http.StatusBadRequest,
		},
		{
			descr: "wrong-entry-type",
			chain: []string{testonly.TestCertPEM},
			want:  http.StatusBadRequest,
		},
		{
			descr:     "backend-rpc-fail",
			chain:     []string{testonly.PrecertPEMValid, testonly.CACertPEM},
			toSign:    "92ecae1a2dc67a6c5f9c96fa5cab4c2faf27c48505b696dad926f161b0ca675a",
			rpcStatus: trillian.TrillianApiStatusCode_ERROR,
			want:      http.StatusInternalServerError,
		},
		{
			descr:     "success",
			chain:     []string{testonly.PrecertPEMValid, testonly.CACertPEM},
			toSign:    "92ecae1a2dc67a6c5f9c96fa5cab4c2faf27c48505b696dad926f161b0ca675a",
			rpcStatus: trillian.TrillianApiStatusCode_OK,
			want:      http.StatusOK,
		},
	}
	info := setupTest(t, []string{testonly.CACertPEM})
	defer info.mockCtrl.Finish()

	for _, test := range tests {
		pool := loadCertsIntoPoolOrDie(t, test.chain)
		chain := createJSONChain(t, *pool)
		if len(test.toSign) > 0 {
			info.expectSign(test.toSign)
			merkleLeaf, _, err := signV1SCTForPrecertificate(info.km, pool.RawCertificates()[0], pool.RawCertificates()[1], fakeTime)
			if err != nil {
				t.Errorf("Unexpected error signing SCT: %v", err)
				continue
			}
			leaves := logLeavesForCert(t, info.km, pool.RawCertificates(), merkleLeaf, true)
			info.client.EXPECT().QueueLeaves(deadlineMatcher(), &trillian.QueueLeavesRequest{LogId: 0x42, Leaves: leaves}).Return(&trillian.QueueLeavesResponse{Status: &trillian.TrillianApiStatus{StatusCode: test.rpcStatus}}, nil)
		}

		recorder := makeAddPrechainRequest(t, info.c, chain)
		if recorder.Code != test.want {
			t.Errorf("addPrechain(%s)=%d (body:%v); want %dv", test.descr, recorder.Code, recorder.Body, test.want)
			continue
		}
		if test.want == http.StatusOK {
			var resp ct.AddChainResponse
			if err := json.NewDecoder(recorder.Body).Decode(&resp); err != nil {
				t.Fatalf("json.Decode(%s)=%v; want nil", recorder.Body.Bytes(), err)
			}

			if got, want := ct.Version(resp.SCTVersion), ct.V1; got != want {
				t.Errorf("resp.SCTVersion=%v; want %v", got, want)
			}
			if got, want := hex.EncodeToString(resp.ID), ctMockLogID; got != want {
				t.Errorf("resp.ID=%s; want %s", got, want)
			}
			if got, want := resp.Timestamp, uint64(1469185273000); got != want {
				t.Errorf("resp.Timestamp=%d; want %d", got, want)
			}
			if got, want := hex.EncodeToString(resp.Signature), "040300067369676e6564"; got != want {
				t.Errorf("resp.Signature=%s; want %s", got, want)
			}
		}
	}
}

func TestGetSTH(t *testing.T) {
	var tests = []struct {
		descr      string
		rpcRsp     *trillian.GetLatestSignedLogRootResponse
		rpcErr     error
		toSign     string // hex-encoded
		signResult []byte
		signErr    error
		want       int
		errStr     string
	}{
		{
			descr:  "backend-failure",
			rpcErr: errors.New("backendfailure"),
			want:   http.StatusInternalServerError,
			errStr: "request failed",
		},
		{
			descr:  "bad-tree-size",
			rpcRsp: makeGetRootResponseForTest(12345, -50, []byte("abcdabcdabcdabcdabcdabcdabcdabcd")),
			want:   http.StatusInternalServerError,
			errStr: "bad tree size",
		},
		{
			descr:  "bad-hash",
			rpcRsp: makeGetRootResponseForTest(12345, 25, []byte("thisisnot32byteslong")),
			want:   http.StatusInternalServerError,
			errStr: "bad hash size",
		},
		{
			descr:      "signer-fail",
			rpcRsp:     makeGetRootResponseForTest(12345, 25, []byte("abcdabcdabcdabcdabcdabcdabcdabcd")),
			want:       http.StatusInternalServerError,
			signResult: []byte{},
			signErr:    errors.New("signerfails"),
			errStr:     "signerfails",
		},
		{
			descr:      "ok",
			rpcRsp:     makeGetRootResponseForTest(12345000000, 25, []byte("abcdabcdabcdabcdabcdabcdabcdabcd")),
			toSign:     "1e88546f5157bfaf77ca2454690b602631fedae925bbe7cf708ea275975bfe74",
			want:       http.StatusOK,
			signResult: []byte{},
		},
	}
	info := setupTest(t, []string{testonly.CACertPEM})
	defer info.mockCtrl.Finish()

	for _, test := range tests {
		info.client.EXPECT().GetLatestSignedLogRoot(deadlineMatcher(), &trillian.GetLatestSignedLogRootRequest{LogId: 0x42}).Return(test.rpcRsp, test.rpcErr)
		req, err := http.NewRequest("GET", "http://example.com/ct/v1/get-sth", nil)
		if err != nil {
			t.Errorf("Failed to create request: %v", err)
			continue
		}
		if len(test.toSign) > 0 {
			info.expectSign(test.toSign)
		} else if test.signResult != nil || test.signErr != nil {
			signer := crypto.NewMockSigner(info.mockCtrl)
			signer.EXPECT().Sign(gomock.Any(), gomock.Any(), gomock.Any()).Return(test.signResult, test.signErr)
			info.km.EXPECT().Signer().Return(signer, nil)
		}
		handler := appHandler{context: info.c, handler: getSTH, name: "GetSTH", method: http.MethodGet}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if got := w.Code; got != test.want {
			t.Errorf("GetSTH(%s).Code=%d; want %d", test.descr, got, test.want)
		}
		if test.errStr != "" {
			if body := w.Body.String(); !strings.Contains(body, test.errStr) {
				t.Errorf("GetSTH(%s)=%q; want to find %q", test.descr, body, test.errStr)
			}
			continue
		}

		var rsp ct.GetSTHResponse
		if err := json.Unmarshal(w.Body.Bytes(), &rsp); err != nil {
			t.Errorf("Failed to unmarshal json response: %s", w.Body.Bytes())
			continue
		}

		if got, want := rsp.TreeSize, uint64(25); got != want {
			t.Errorf("GetSTH(%s).TreeSize=%d; want %d", test.descr, got, want)
		}
		if got, want := rsp.Timestamp, uint64(12345); got != want {
			t.Errorf("GetSTH(%s).Timestamp=%d; want %d", test.descr, got, want)
		}
		if got, want := hex.EncodeToString(rsp.SHA256RootHash), "6162636461626364616263646162636461626364616263646162636461626364"; got != want {
			t.Errorf("GetSTH(%s).SHA256RootHash=%s; want %s", test.descr, got, want)
		}
		if got, want := hex.EncodeToString(rsp.TreeHeadSignature), "040300067369676e6564"; got != want {
			t.Errorf("GetSTH(%s).TreeHeadSignature=%s; want %s", test.descr, got, want)
		}
	}
}

func TestGetEntries(t *testing.T) {
	// Create a couple of valid serialized ct.MerkleTreeLeaf objects
	merkleLeaf1 := ct.MerkleTreeLeaf{
		Version:  ct.V1,
		LeafType: ct.TimestampedEntryLeafType,
		TimestampedEntry: &ct.TimestampedEntry{
			Timestamp:  12345,
			EntryType:  ct.X509LogEntryType,
			X509Entry:  &ct.ASN1Cert{Data: []byte("certdatacertdata")},
			Extensions: ct.CTExtensions{},
		},
	}
	merkleLeaf2 := ct.MerkleTreeLeaf{
		Version:  ct.V1,
		LeafType: ct.TimestampedEntryLeafType,
		TimestampedEntry: &ct.TimestampedEntry{
			Timestamp:  67890,
			EntryType:  ct.X509LogEntryType,
			X509Entry:  &ct.ASN1Cert{Data: []byte("certdat2certdat2")},
			Extensions: ct.CTExtensions{},
		},
	}
	merkleBytes1, err1 := tls.Marshal(merkleLeaf1)
	merkleBytes2, err2 := tls.Marshal(merkleLeaf2)
	if err1 != nil || err2 != nil {
		t.Fatalf("failed to tls.Marshal() test data for get-entries: %v %v", err1, err2)
	}

	var tests = []struct {
		descr  string
		req    string
		want   int
		rpcRsp *trillian.GetLeavesByIndexResponse
		rpcErr error
		errStr string
	}{
		{
			descr: "invalid &&s",
			req:   "start=&&&&&&&&&end=wibble",
			want:  http.StatusBadRequest,
		},
		{
			descr: "start non numeric",
			req:   "start=fish&end=3",
			want:  http.StatusBadRequest,
		},
		{
			descr: "end non numeric",
			req:   "start=10&end=wibble",
			want:  http.StatusBadRequest,
		},
		{
			descr: "both non numeric",
			req:   "start=fish&end=wibble",
			want:  http.StatusBadRequest,
		},
		{
			descr: "end missing",
			req:   "start=1",
			want:  http.StatusBadRequest,
		},
		{
			descr: "start missing",
			req:   "end=1",
			want:  http.StatusBadRequest,
		},
		{
			descr: "both missing",
			req:   "",
			want:  http.StatusBadRequest,
		},
		{
			descr:  "backend rpc error",
			req:    "start=1&end=2",
			want:   http.StatusInternalServerError,
			rpcErr: errors.New("bang"),
			errStr: "bang",
		},
		{
			descr: "backend extra leaves",
			req:   "start=1&end=2",
			want:  http.StatusInternalServerError,
			rpcRsp: &trillian.GetLeavesByIndexResponse{
				Status: okStatus,
				Leaves: []*trillian.LogLeaf{{LeafIndex: 1}, {LeafIndex: 2}, {LeafIndex: 3}},
			},
			errStr: "too many leaves",
		},
		{
			descr: "backend non-contiguous range",
			req:   "start=1&end=2",
			want:  http.StatusInternalServerError,
			rpcRsp: &trillian.GetLeavesByIndexResponse{
				Status: okStatus,
				Leaves: []*trillian.LogLeaf{{LeafIndex: 1}, {LeafIndex: 3}},
			},
			errStr: "unexpected leaf index",
		},
		{
			descr: "backend leaf corrupt",
			req:   "start=1&end=2",
			want:  http.StatusOK,
			rpcRsp: &trillian.GetLeavesByIndexResponse{
				Status: okStatus,
				Leaves: []*trillian.LogLeaf{
					{LeafIndex: 1, MerkleLeafHash: []byte("hash"), LeafValue: []byte("NOT A MERKLE TREE LEAF")},
					{LeafIndex: 2, MerkleLeafHash: []byte("hash"), LeafValue: []byte("NOT A MERKLE TREE LEAF")},
				},
			},
		},
		{
			descr: "leaves ok",
			req:   "start=1&end=2",
			want:  http.StatusOK,
			rpcRsp: &trillian.GetLeavesByIndexResponse{
				Status: okStatus,
				Leaves: []*trillian.LogLeaf{
					{LeafIndex: 1, MerkleLeafHash: []byte("hash"), LeafValue: merkleBytes1, ExtraData: []byte("extra1")},
					{LeafIndex: 2, MerkleLeafHash: []byte("hash"), LeafValue: merkleBytes2, ExtraData: []byte("extra2")},
				},
			},
		},
	}
	info := setupTest(t, nil)
	defer info.mockCtrl.Finish()
	handler := appHandler{context: info.c, handler: getEntries, name: "GetEntries", method: http.MethodGet}

	for _, test := range tests {
		path := fmt.Sprintf("/ct/v1/get-entries?%s", test.req)
		req, err := http.NewRequest("GET", path, nil)
		if err != nil {
			t.Errorf("Failed to create request: %v", err)
			continue
		}
		if test.rpcRsp != nil || test.rpcErr != nil {
			info.client.EXPECT().GetLeavesByIndex(deadlineMatcher(), &trillian.GetLeavesByIndexRequest{LogId: 0x42, LeafIndex: []int64{1, 2}}).Return(test.rpcRsp, test.rpcErr)
		}

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if got := w.Code; got != test.want {
			t.Errorf("GetEntries(%q)=%d; want %d (because %s)", test.req, got, test.want, test.descr)
		}
		if test.errStr != "" {
			if body := w.Body.String(); !strings.Contains(body, test.errStr) {
				t.Errorf("GetEntries(%q)=%q; want to find %q (because %s)", test.req, body, test.errStr, test.descr)
			}
			continue
		}
		if test.want != http.StatusOK {
			continue
		}
		// Leaf data should be passed through as-is even if invalid.
		var jsonMap map[string][]ct.LeafEntry
		if err := json.Unmarshal(w.Body.Bytes(), &jsonMap); err != nil {
			t.Errorf("Failed to unmarshal json response %s: %v", w.Body.Bytes(), err)
			continue
		}
		if got := len(jsonMap); got != 1 {
			t.Errorf("len(rspMap)=%d; want 1", got)
		}
		entries := jsonMap["entries"]
		if got, want := len(entries), len(test.rpcRsp.Leaves); got != want {
			t.Errorf("len(rspMap['entries']=%d; want %d", got, want)
			continue
		}
		for i := 0; i < len(entries); i++ {
			if got, want := string(entries[i].LeafInput), string(test.rpcRsp.Leaves[i].LeafValue); got != want {
				t.Errorf("rspMap['entries'][%d].LeafInput=%s; want %s", i, got, want)
			}
			if got, want := string(entries[i].ExtraData), string(test.rpcRsp.Leaves[i].ExtraData); got != want {
				t.Errorf("rspMap['entries'][%d].ExtraData=%s; want %s", i, got, want)
			}
		}
	}
}

func TestGetEntriesRanges(t *testing.T) {
	var tests = []struct {
		start int64
		end   int64
		want  int
		desc  string
		rpc   bool
	}{
		{-1, 0, http.StatusBadRequest, "-ve start value not allowed", false},
		{0, -1, http.StatusBadRequest, "-ve end value not allowed", false},
		{20, 10, http.StatusBadRequest, "invalid range end>start", false},
		{3000, -50, http.StatusBadRequest, "invalid range, -ve end", false},
		{10, 20, http.StatusInternalServerError, "valid range", true},
		{10, 10, http.StatusInternalServerError, "valid range, one entry", true},
		{10, 9, http.StatusBadRequest, "invalid range, edge case", false},
		{1000, 50000, http.StatusBadRequest, "range too large to be accepted", false},
	}

	info := setupTest(t, nil)
	defer info.mockCtrl.Finish()
	handler := appHandler{context: info.c, handler: getEntries, name: "GetEntries", method: http.MethodGet}

	// This tests that only valid ranges make it to the backend for get-entries.
	// We're testing request handling up to the point where we make the RPC so arrange for
	// it to fail with a specific error.
	for _, test := range tests {
		if test.rpc {
			info.client.EXPECT().GetLeavesByIndex(deadlineMatcher(), &trillian.GetLeavesByIndexRequest{LogId: 0x42, LeafIndex: buildIndicesForRange(test.start, test.end)}).Return(nil, errors.New("RPCMADE"))
		}

		path := fmt.Sprintf("/ct/v1/get-entries?start=%d&end=%d", test.start, test.end)
		req, err := http.NewRequest("GET", path, nil)
		if err != nil {
			t.Errorf("Failed to create request: %v", err)
			continue
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if got := w.Code; got != test.want {
			t.Errorf("getEntries(%d, %d)=%d; want %d for test %s", test.start, test.end, got, test.want, test.desc)
		}
		if test.rpc && !strings.Contains(w.Body.String(), "RPCMADE") {
			// If an RPC was emitted, it should have received and propagated an error.
			t.Errorf("getEntries(%d, %d)=%q; expect RPCMADE for test %s", test.start, test.end, w.Body, test.desc)
		}
	}
}

func TestSortLeafRange(t *testing.T) {
	var tests = []struct {
		start   int64
		end     int64
		entries []int
		errStr  string
	}{
		{1, 2, []int{1, 2}, ""},
		{1, 1, []int{1}, ""},
		{5, 12, []int{5, 6, 7, 8, 9, 10, 11, 12}, ""},
		{5, 12, []int{5, 6, 7, 8, 9, 10}, ""},
		{5, 12, []int{7, 6, 8, 9, 10, 5}, ""},
		{5, 12, []int{5, 5, 6, 7, 8, 9, 10}, "unexpected leaf index"},
		{5, 12, []int{6, 7, 8, 9, 10, 11, 12}, "unexpected leaf index"},
		{5, 12, []int{5, 6, 7, 8, 9, 10, 12}, "unexpected leaf index"},
		{5, 12, []int{5, 6, 7, 8, 9, 10, 11, 12, 13}, "too many leaves"},
		{1, 4, []int{5, 2, 3}, "unexpected leaf index"},
	}
	for _, test := range tests {
		rsp := trillian.GetLeavesByIndexResponse{}
		for _, idx := range test.entries {
			rsp.Leaves = append(rsp.Leaves, &trillian.LogLeaf{LeafIndex: int64(idx)})
		}
		err := sortLeafRange(&rsp, test.start, test.end)
		if test.errStr != "" {
			if err == nil {
				t.Errorf("sortLeafRange(%v, %d, %d)=nil; want substring %q", test.entries, test.start, test.end, test.errStr)
			} else if !strings.Contains(err.Error(), test.errStr) {
				t.Errorf("sortLeafRange(%v, %d, %d)=%v; want substring %q", test.entries, test.start, test.end, err, test.errStr)
			}
			continue
		}
		if err != nil {
			t.Errorf("sortLeafRange(%v, %d, %d)=%v; want nil", test.entries, test.start, test.end, err)
		}
	}
}

func TestGetProofByHash(t *testing.T) {
	inclusionProof := ct.GetProofByHashResponse{
		LeafIndex: 2,
		AuditPath: [][]byte{[]byte("abcdef"), []byte("ghijkl"), []byte("mnopqr")},
	}

	var tests = []struct {
		req    string
		want   int
		rpcRsp *trillian.GetInclusionProofByHashResponse
		rpcErr error
		errStr string
	}{
		{
			req:  "",
			want: http.StatusBadRequest,
		},
		{
			req:  "hash=&tree_size=1",
			want: http.StatusBadRequest,
		},
		{
			req:  "hash=''&tree_size=1",
			want: http.StatusBadRequest,
		},
		{
			req:  "hash=notbase64data&tree_size=1",
			want: http.StatusBadRequest,
		},
		{
			req:  "tree_size=-1&hash=aGkK",
			want: http.StatusBadRequest,
		},
		{
			req:    "tree_size=6&hash=YWhhc2g=",
			want:   http.StatusInternalServerError,
			rpcErr: errors.New("RPCFAIL"),
			errStr: "RPCFAIL",
		},
		{
			req:  "tree_size=7&hash=YWhhc2g=",
			want: http.StatusOK,
			rpcRsp: &trillian.GetInclusionProofByHashResponse{
				Status: okStatus,
				Proof: []*trillian.Proof{
					&trillian.Proof{
						LeafIndex: 2,
						// Proof to match inclusionProof above.
						ProofNode: []*trillian.Node{
							{NodeHash: []byte("abcdef")},
							{NodeHash: []byte("ghijkl")},
							{NodeHash: []byte("mnopqr")},
						},
					},
					// Second proof ignored.
					&trillian.Proof{
						LeafIndex: 2,
						ProofNode: []*trillian.Node{
							{NodeHash: []byte("ghijkl")},
						},
					},
				},
			},
		},
		{
			req:  "tree_size=9&hash=YWhhc2g=",
			want: http.StatusInternalServerError,
			rpcRsp: &trillian.GetInclusionProofByHashResponse{
				Status: okStatus,
				Proof: []*trillian.Proof{
					&trillian.Proof{
						LeafIndex: 2,
						ProofNode: []*trillian.Node{
							{NodeHash: []byte("abcdef")},
							{NodeHash: []byte{}}, // missing hash
							{NodeHash: []byte("ghijkl")},
						},
					},
				},
			},
			errStr: "invalid proof",
		},
		{
			req:  "tree_size=7&hash=YWhhc2g=",
			want: http.StatusOK,
			rpcRsp: &trillian.GetInclusionProofByHashResponse{
				Status: okStatus,
				Proof: []*trillian.Proof{
					&trillian.Proof{
						LeafIndex: 2,
						// Proof to match inclusionProof above.
						ProofNode: []*trillian.Node{
							{NodeHash: []byte("abcdef")},
							{NodeHash: []byte("ghijkl")},
							{NodeHash: []byte("mnopqr")},
						},
					},
				},
			},
		},
	}
	info := setupTest(t, nil)
	defer info.mockCtrl.Finish()
	handler := appHandler{context: info.c, handler: getProofByHash, name: "GetProofByHash", method: http.MethodGet}

	for _, test := range tests {
		req, err := http.NewRequest("GET", fmt.Sprintf("/ct/v1/proof-by-hash?%s", test.req), nil)
		if err != nil {
			t.Errorf("Failed to create request: %v", err)
			continue
		}
		if test.rpcRsp != nil || test.rpcErr != nil {
			info.client.EXPECT().GetInclusionProofByHash(deadlineMatcher(), gomock.Any()).Return(test.rpcRsp, test.rpcErr)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if got := w.Code; got != test.want {
			t.Errorf("proofByHash(%s)=%d; want %d", test.req, got, test.want)
		}
		if test.errStr != "" {
			if body := w.Body.String(); !strings.Contains(body, test.errStr) {
				t.Errorf("proofByHash(%q)=%q; want to find %q", test.req, body, test.errStr)
			}
			continue
		}
		if test.want != http.StatusOK {
			continue
		}
		var resp ct.GetProofByHashResponse
		if err = json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Errorf("Failed to unmarshal json response %s: %v", w.Body.Bytes(), err)
			continue
		}
		if !reflect.DeepEqual(resp, inclusionProof) {
			t.Errorf("proofByHash(%q)=%+v; want %+v", test.req, resp, inclusionProof)
		}
	}
}

func TestGetSTHConsistency(t *testing.T) {
	consistencyProof := ct.GetSTHConsistencyResponse{
		Consistency: [][]byte{[]byte("abcdef"), []byte("ghijkl"), []byte("mnopqr")},
	}
	var tests = []struct {
		req    string
		want   int
		rpcRsp *trillian.GetConsistencyProofResponse
		rpcErr error
		errStr string
	}{
		{
			req:  "",
			want: http.StatusBadRequest,
		},
		{
			req:  "first=apple&second=orange",
			want: http.StatusBadRequest,
		},
		{
			req:  "first=1&second=a",
			want: http.StatusBadRequest,
		},
		{
			req:  "first=a&second=2",
			want: http.StatusBadRequest,
		},
		{
			req:  "first=-1&second=10",
			want: http.StatusBadRequest,
		},
		{
			req:  "first=10&second=-11",
			want: http.StatusBadRequest,
		},
		{
			req:  "first=6&second=6",
			want: http.StatusBadRequest,
		},
		{
			req:  "first=998&second=997",
			want: http.StatusBadRequest,
		},
		{
			req:  "first=1000&second=200",
			want: http.StatusBadRequest,
		},
		{
			req:  "first=10",
			want: http.StatusBadRequest,
		},
		{
			req:  "second=20",
			want: http.StatusBadRequest,
		},
		{
			req:    "first=10&second=20",
			want:   http.StatusInternalServerError,
			rpcErr: errors.New("RPCFAIL"),
			errStr: "RPCFAIL",
		},
		{
			req:  "first=10&second=20",
			want: http.StatusInternalServerError,
			rpcRsp: &trillian.GetConsistencyProofResponse{
				Status: okStatus,
				Proof: &trillian.Proof{
					LeafIndex: 2,
					ProofNode: []*trillian.Node{
						{NodeHash: []byte("abcdef")},
						{NodeHash: []byte{}}, // invalid
						{NodeHash: []byte("ghijkl")},
					},
				},
			},
			errStr: "invalid proof",
		},
		{
			req:  "first=10&second=20",
			want: http.StatusOK,
			rpcRsp: &trillian.GetConsistencyProofResponse{
				Status: okStatus,
				Proof: &trillian.Proof{
					LeafIndex: 2,
					// Proof to match consistencyProof above.
					ProofNode: []*trillian.Node{
						{NodeHash: []byte("abcdef")},
						{NodeHash: []byte("ghijkl")},
						{NodeHash: []byte("mnopqr")},
					},
				},
			},
		},
	}

	info := setupTest(t, nil)
	defer info.mockCtrl.Finish()
	handler := appHandler{context: info.c, handler: getSTHConsistency, name: "GetSTHConsistency", method: http.MethodGet}

	for _, test := range tests {
		req, err := http.NewRequest("GET", fmt.Sprintf("/ct/v1/get-sth-consistency?%s", test.req), nil)
		if err != nil {
			t.Errorf("Failed to create request: %v", err)
			continue
		}
		if test.rpcRsp != nil || test.rpcErr != nil {
			info.client.EXPECT().GetConsistencyProof(deadlineMatcher(), &trillian.GetConsistencyProofRequest{LogId: 0x42, FirstTreeSize: 10, SecondTreeSize: 20}).Return(test.rpcRsp, test.rpcErr)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if got := w.Code; got != test.want {
			t.Errorf("getSTHConsistency(%s)=%d; want %d", test.req, got, test.want)
		}
		if test.errStr != "" {
			if body := w.Body.String(); !strings.Contains(body, test.errStr) {
				t.Errorf("getSTHConsistency(%q)=%q; want to find %q", test.req, body, test.errStr)
			}
			continue
		}
		if test.want != http.StatusOK {
			continue
		}
		var resp ct.GetSTHConsistencyResponse
		if err = json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Errorf("Failed to unmarshal json response %s: %v", w.Body.Bytes(), err)
			continue
		}
		if !reflect.DeepEqual(resp, consistencyProof) {
			t.Errorf("getSTHConsistency(%q)=%+v; want %+v", test.req, resp, consistencyProof)
		}
	}
}

func TestGetEntryAndProof(t *testing.T) {
	merkleLeaf := ct.MerkleTreeLeaf{
		Version:  ct.V1,
		LeafType: ct.TimestampedEntryLeafType,
		TimestampedEntry: &ct.TimestampedEntry{
			Timestamp:  12345,
			EntryType:  ct.X509LogEntryType,
			X509Entry:  &ct.ASN1Cert{Data: []byte("certdatacertdata")},
			Extensions: ct.CTExtensions{},
		},
	}
	leafBytes, err := tls.Marshal(merkleLeaf)
	if err != nil {
		t.Fatalf("failed to build test Merkle leaf data: %v", err)
	}

	var tests = []struct {
		req    string
		want   int
		rpcRsp *trillian.GetEntryAndProofResponse
		rpcErr error
		errStr string
	}{
		{
			req:  "",
			want: http.StatusBadRequest,
		},
		{
			req:  "leaf_index=b",
			want: http.StatusBadRequest,
		},
		{
			req:  "leaf_index=1&tree_size=-1",
			want: http.StatusBadRequest,
		},
		{
			req:  "leaf_index=-1&tree_size=1",
			want: http.StatusBadRequest,
		},
		{
			req:  "leaf_index=1&tree_size=d",
			want: http.StatusBadRequest,
		},
		{
			req:  "leaf_index=&tree_size=",
			want: http.StatusBadRequest,
		},
		{
			req:  "leaf_index=",
			want: http.StatusBadRequest,
		},
		{
			req:  "leaf_index=1&tree_size=0",
			want: http.StatusBadRequest,
		},
		{
			req:  "leaf_index=10&tree_size=5",
			want: http.StatusBadRequest,
		},
		{
			req:  "leaf_index=tree_size",
			want: http.StatusBadRequest,
		},
		{
			req:    "leaf_index=1&tree_size=3",
			want:   http.StatusInternalServerError,
			rpcErr: errors.New("RPCFAIL"),
			errStr: "RPCFAIL",
		},
		{
			req:  "leaf_index=1&tree_size=3",
			want: http.StatusInternalServerError,
			// No result data in backend response
			rpcRsp: &trillian.GetEntryAndProofResponse{Status: okStatus},
		},
		{
			req:  "leaf_index=1&tree_size=3",
			want: http.StatusOK,
			rpcRsp: &trillian.GetEntryAndProofResponse{
				Status: okStatus,
				Proof: &trillian.Proof{
					LeafIndex: 2,
					ProofNode: []*trillian.Node{
						{NodeHash: []byte("abcdef")},
						{NodeHash: []byte("ghijkl")},
						{NodeHash: []byte("mnopqr")},
					},
				},
				// To match merkleLeaf above.
				Leaf: &trillian.LogLeaf{
					LeafValue:      leafBytes,
					MerkleLeafHash: []byte("ahash"),
					ExtraData:      []byte("extra"),
				},
			},
		},
	}

	info := setupTest(t, nil)
	defer info.mockCtrl.Finish()
	handler := appHandler{context: info.c, handler: getEntryAndProof, name: "GetEntryAndProof", method: http.MethodGet}

	for _, test := range tests {
		req, err := http.NewRequest("GET", fmt.Sprintf("/ct/v1/get-entry-and-proof?%s", test.req), nil)
		if err != nil {
			t.Errorf("Failed to create request: %v", err)
			continue
		}

		if test.rpcRsp != nil || test.rpcErr != nil {
			info.client.EXPECT().GetEntryAndProof(deadlineMatcher(), &trillian.GetEntryAndProofRequest{LogId: 0x42, LeafIndex: 1, TreeSize: 3}).Return(test.rpcRsp, test.rpcErr)
		}

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if got := w.Code; got != test.want {
			t.Errorf("getEntryAndProof(%s)=%d; want %d", test.req, got, test.want)
		}
		if test.errStr != "" {
			if body := w.Body.String(); !strings.Contains(body, test.errStr) {
				t.Errorf("getEntryAndProof(%q)=%q; want to find %q", test.req, body, test.errStr)
			}
			continue
		}
		if test.want != http.StatusOK {
			continue
		}

		var resp ct.GetEntryAndProofResponse
		if err = json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Errorf("Failed to unmarshal json response %s: %v", w.Body.Bytes(), err)
			continue
		}
		// The result we expect after a roundtrip in the successful get entry and proof test
		wantRsp := ct.GetEntryAndProofResponse{
			LeafInput: leafBytes,
			ExtraData: []byte("extra"),
			AuditPath: [][]byte{[]byte("abcdef"), []byte("ghijkl"), []byte("mnopqr")},
		}
		if !reflect.DeepEqual(resp, wantRsp) {
			t.Errorf("getEntryAndProof(%q)=%+v; want %+v", test.req, resp, wantRsp)
		}
	}
}

func createJSONChain(t *testing.T, p PEMCertPool) io.Reader {
	var req ct.AddChainRequest
	for _, rawCert := range p.RawCertificates() {
		req.Chain = append(req.Chain, rawCert.Raw)
	}

	var buffer bytes.Buffer
	// It's tempting to avoid creating and flushing the intermediate writer but it doesn't work
	writer := bufio.NewWriter(&buffer)
	err := json.NewEncoder(writer).Encode(&req)
	writer.Flush()

	if err != nil {
		t.Fatalf("Failed to create test json: %v", err)
	}

	return bufio.NewReader(&buffer)
}

func logLeavesForCert(t *testing.T, km crypto.KeyManager, certs []*x509.Certificate, merkleLeaf ct.MerkleTreeLeaf, isPrecert bool) []*trillian.LogLeaf {
	leafData, err := tls.Marshal(merkleLeaf)
	if err != nil {
		t.Fatalf("failed to serialize leaf: %v", err)
	}

	// This is a hash of the leaf data, not the the Merkle hash as defined in the RFC.
	leafHash := sha256.Sum256(leafData)

	extraData, err := extraDataForChain(certs, isPrecert)
	if err != nil {
		t.Fatalf("failed to serialize extra data: %v", err)
	}

	return []*trillian.LogLeaf{{LeafValueHash: leafHash[:], LeafValue: leafData, ExtraData: extraData}}
}

type dlMatcher struct {
}

func deadlineMatcher() gomock.Matcher {
	return dlMatcher{}
}

func (d dlMatcher) Matches(x interface{}) bool {
	ctx, ok := x.(context.Context)
	if !ok {
		return false
	}

	deadlineTime, ok := ctx.Deadline()
	if !ok {
		return false // we never make RPC calls without a deadline set
	}

	return deadlineTime == fakeDeadlineTime
}

func (d dlMatcher) String() string {
	return fmt.Sprintf("deadline is %v", fakeDeadlineTime)
}

func makeAddPrechainRequest(t *testing.T, c LogContext, body io.Reader) *httptest.ResponseRecorder {
	handler := appHandler{context: c, handler: addPreChain, name: "AddPreChain", method: http.MethodPost}
	return makeAddChainRequestInternal(t, handler, "add-pre-chain", body)
}

func makeAddChainRequest(t *testing.T, c LogContext, body io.Reader) *httptest.ResponseRecorder {
	handler := appHandler{context: c, handler: addChain, name: "AddChain", method: http.MethodPost}
	return makeAddChainRequestInternal(t, handler, "add-chain", body)
}

func makeAddChainRequestInternal(t *testing.T, handler appHandler, path string, body io.Reader) *httptest.ResponseRecorder {
	req, err := http.NewRequest("POST", fmt.Sprintf("http://example.com/ct/v1/%s", path), body)
	if err != nil {
		t.Fatalf("Failed to create POST request: %v", err)
	}

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	return w
}

func bytesToLeaf(leafBytes []byte) (*ct.MerkleTreeLeaf, error) {
	var treeLeaf ct.MerkleTreeLeaf
	if _, err := tls.Unmarshal(leafBytes, &treeLeaf); err != nil {
		return nil, err
	}
	return &treeLeaf, nil
}

func makeGetRootResponseForTest(stamp, treeSize int64, hash []byte) *trillian.GetLatestSignedLogRootResponse {
	return &trillian.GetLatestSignedLogRootResponse{
		Status: &trillian.TrillianApiStatus{StatusCode: trillian.TrillianApiStatusCode_OK},
		SignedLogRoot: &trillian.SignedLogRoot{
			TimestampNanos: stamp,
			TreeSize:       treeSize,
			RootHash:       hash,
		},
	}
}

func loadCertsIntoPoolOrDie(t *testing.T, certs []string) *PEMCertPool {
	pool := NewPEMCertPool()
	for _, cert := range certs {
		if !pool.AppendCertsFromPEM([]byte(cert)) {
			t.Fatalf("couldn't parse test certs: %v", certs)
		}
	}
	return pool
}
