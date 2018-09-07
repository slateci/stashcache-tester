/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.

You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type TestSet struct {
	DNSName     string   `json:"dnsname"`
	SiteName    string   `json:"sitename"`
	HashFile    string   `json:"hashfile"`
	TestSetName string   `json:"testsetname"`
	TestFiles   []string `json:"testfiles"`
}

type ESPayload struct {
	Cache            string  `json:cache`
	DestinationSpace string  `json:destination_space`
	DownloadSize     int64   `json:download_size`
	DownloadTime     float64 `json:download_time`
	End1             int64   `json:end1`
	End2             int64   `json:end2`
	End3             int64   `json:end3`
	FileName         string  `json:filename`
	FileSize         int64   `json:filesize`
	Host             string  `json:host`
	SiteName         string  `json:sitename`
	Start1           int64   `json:start1`
	Start2           int64   `json:start2`
	Start3           int64   `json:start3`
	Status           string  `json:status`
	TimeStamp        int64   `json:timestamp`
	Tries            int     `json:tries`
	XRDcpVersion     string  `json:xrdcp_version`
	XRDExit1         string  `json:xrdexit1`
	XRDExit2         string  `json:xrdexit2`
}

const ESCollector = "http://uct2-collectd.mwt2.org:9951"

func decodeJSON(configLocation string) (map[string][]TestSet, error) {
	decodedConfig := make(map[string][]TestSet)
	fileContents, err := ioutil.ReadFile(configLocation)
	if err != nil {
		log.Fatalf("Can't read config file %s, got error %s\n", configLocation, err)
	}
	var rawConfig []TestSet
	err = json.Unmarshal(fileContents, &rawConfig)
	if err != nil {
		log.Fatal("Can't decode json from config file")
	}
	for _, val := range rawConfig {
		if entry, ok := decodedConfig[val.SiteName]; ok {
			decodedConfig[val.SiteName] = append(entry, val)
		} else {
			entry = []TestSet{val}
			decodedConfig[val.SiteName] = entry
		}

	}
	return decodedConfig, nil
}

func TestDataSet(ts TestSet, resultChan chan bool) {

	workingDir, err := ioutil.TempDir(".", "")
	if err != nil {
		fmt.Printf("Couldn't create directory for %s\n", workingDir)
	}
	defer os.RemoveAll(workingDir)

	curDir, err := os.Getwd()
	if err != nil {
		fmt.Println("Couldn't get current directory")
		resultChan <- false
		return
	}
	defer os.Chdir(curDir)
	if err := os.Chdir(workingDir); err != nil {
		fmt.Println("Can't change to working directory")
		resultChan <- false
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()

	var out bytes.Buffer
	for _, remoteFile := range ts.TestFiles {
		// Setup context to terminate commands after 600 seconds

		var payload ESPayload

		origURI := "root://" + ts.DNSName + "/" + remoteFile
		cmd := exec.CommandContext(ctx, "xrdcp", origURI, ".")
		//  populate payload info to report to ES
		payload.XRDcpVersion = "stashcache-tester"
		payload.SiteName = ts.SiteName
		payload.FileName = filepath.Base(remoteFile)
		payload.Cache = ts.DNSName
		payload.Host = ts.DNSName
		start := time.Now()
		payload.Start1 = start.Unix() * 1000 // need to multiple by 1000 for ES
		payload.Tries = 1
		cmd.Stdout = &out
		cmd.Env = append(os.Environ(),
			"XRD_REQUESTTIMEOUT=30",   // Wait 30s before timing out
			"XRD_CPCHUNKSIZE=8388608", // read 8MB at a time
			"XRD_TIMEOUTRESOLUTION=5", // Check for timeouts every 5s
			"XRD_CONNECTIONWINDOW=30", // Wait 30s for initial TCP connection
			"XRD_CONNECTIONRETRY=2",   // Retry 2 times
			"XRD_STREAMTIMEOUT=30")    // Wait 30s for TCP activity

		if err := cmd.Run(); err != nil {
			end := time.Now()
			payload.End1 = end.Unix() * 1000 // need to multiple by 1000 for ES
			payload.DownloadTime = end.Sub(start).Seconds() * 1000
			payload.DownloadSize = 0
			payload.TimeStamp = time.Now().Unix() * 1000 // need to multiple by 1000 for ES
			payload.Status = "Failure"

			fmt.Printf("Can't download %s\nError: %s\n", origURI, err)
			ReportTest(payload)
			resultChan <- false
			return
		} else {
			payload.Status = "Success"
			payload.XRDExit1 = "0"

		}
		end := time.Now()
		payload.End1 = end.Unix() * 1000 // need to multiple by 1000 for ES
		payload.DownloadTime = end.Sub(start).Seconds() * 1000

		if fileInfo, err := os.Stat(payload.FileName); err != nil {
			payload.DownloadSize = 0
			payload.TimeStamp = time.Now().Unix() * 1000 // need to multiple by 1000 for ES
			ReportTest(payload)
			resultChan <- false
			return
		} else {
			payload.DownloadSize = fileInfo.Size()
			payload.FileSize = fileInfo.Size()
			payload.TimeStamp = time.Now().Unix() * 1000 // need to multiple by 1000 for ES
		}
		ReportTest(payload)
	}
	cmd := exec.CommandContext(ctx, "sha256sum", "-c", "hashes")
	cmd.Stdout = &out
	err = cmd.Run()
	if err != nil {
		fmt.Printf("Can't verify file hashes: %s\n", err)
		resultChan <- false

	}
	resultChan <- true
}

func TestEndpoint(testsets []TestSet, c chan bool) {
	workDir, err := ioutil.TempDir("", "")
	testsSucceeded := true
	if err != nil {
		fmt.Println("Couldn't create test directory: ", err)
		c <- false
		return
	}
	defer os.RemoveAll(workDir)
	curDir, err := os.Getwd()
	if err != nil {
		fmt.Println("Couldn't get current directory", workDir)
		c <- false
		return
	}
	if os.Chdir(workDir) != nil {
		c <- false
		return
	}

	testResultChan := make(chan bool)
	for _, ts := range testsets {
		go TestDataSet(ts, testResultChan)
		success := <-testResultChan
		testsSucceeded = testsSucceeded && success
		if !success {
			fmt.Printf("Failed to verify %s using endpoint %s\n", ts.TestSetName, ts.SiteName)
		}
	}

	if os.Chdir(curDir) != nil {
		c <- false
		return
	}
	c <- testsSucceeded
}

func ReportTest(payload ESPayload) {
	return
	//fmt.Printf("\n ******\n Payload: %+v \n", payload)
	//buf := new(bytes.Buffer)
	//json.NewEncoder(buf).Encode(payload)
	//_, err := http.Post(ESCollector, "application/json", buf)
	//if err != nil {
	//	fmt.Printf("Error reporting test results to ES collector\n")
	//}

}

func main() {

	c := make(chan bool)
	var testSets map[string][]TestSet
	var err error
	if testSets, err = decodeJSON("siteconfig.json"); err != nil {
		panic("Can't read config file")
	}
	for k, v := range testSets {
		fmt.Printf("Testing endpoint %s\n", k)
		go TestEndpoint(v, c)
		success := <-c
		if !success {
			fmt.Printf("%s failed testing\n", k)
		} else {
			fmt.Printf("%s passed testing\n", k)
		}
	}

}
