package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func init() {
	http.HandleFunc("/launcher", launcherHandler)
	//http.HandleFunc("/mount", mountHandler)
}

func TestLauncherHandler(t *testing.T) {
	testCases := []struct {
		caseType   string
		command    string
		statusCode int
	}{
		{
			caseType:   "normal",
			command:    "echo \"123\"",
			statusCode: http.StatusOK,
		},
		{
			caseType:   "abnormal",
			command:    "1234567",
			statusCode: http.StatusInternalServerError,
		},
		{
			caseType:   "bodyNoFieldCommand",
			statusCode: http.StatusBadRequest,
		},
		{
			caseType:   "bodyNotJson",
			statusCode: http.StatusInternalServerError,
		},
	}

	for _, tc := range testCases {
		t.Logf("When checking %s for status code %d", tc.command, tc.statusCode)

		body := make(map[string]string)
		if tc.command != "" {
			body["command"] = tc.command
		}

		if tc.caseType == "bodyNoFieldCommand" {
			body["test"] = "test"
		}

		bbody, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("\t\tfind error %v", err)
			return
		}

		requestbody := strings.NewReader(string(bbody))
		if tc.caseType == "bodyNotJson" {
			requestbody = strings.NewReader("")
		}

		req, err := http.NewRequest("POST", "/launcher", requestbody)
		if err != nil {
			t.Fatalf("\t\tfind error %v", err)
			return
		}

		rw := httptest.NewRecorder()
		http.DefaultServeMux.ServeHTTP(rw, req)

		if rw.Code == tc.statusCode {
			t.Logf("\t\tGetting the expect statuscode(%v) for command(%s) and casetype(%s)", tc.statusCode, tc.command, tc.caseType)
		} else {
			t.Errorf("\t\tGetting a unexpect statuscode(%v) for command(%s) and casetype(%s), we expect %v", rw.Code, tc.command, tc.caseType, tc.statusCode)
		}
	}
}
