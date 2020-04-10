package rest

import (
	"io/ioutil"
	"net/http"
)

func fetchURL(url string) (interface{}, int, string) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, http.StatusBadRequest, "Unable to fetch provider"
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, http.StatusBadRequest, "Error in parsing data"
	}

	if resp.StatusCode >= 400 {
		return nil, resp.StatusCode, string(body)
	}
	return body, http.StatusOK, ""
}
