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
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"time"
)

var endpoints = []string{"stashcache.grid.uchicago.edu", "hcc-stash.unl.edu", "192.170.227.239"}
var testSetDirs = []string{"/user/sthapa/public/test-sets/",
	"/user/sthapa/public/test-sets/filetest/"}
var testSets  = make(map[string][]string)

func populateTestSets(testSets map[string][]string) {
	for _, testDir := range testSetDirs {
		switch testDir {
		case "/user/sthapa/public/test-sets/":
			testSets[testDir] = []string{testDir + "/test.1M",
				testDir + "/test.100M",
				testDir + "/test.1024M",
				testDir + "/hashes"}
		case "/user/sthapa/public/test-sets/filetest/":
			fileList := make([]string, 101)
			for i := 0; i < 100; i++ {
				fileList[i] = fmt.Sprintf("/user/sthapa/test-sets/filetest/test_file.%d", i+1)
			}
			fileList[100] = "/user/sthapa/test-sets/filetest/hashes"
			testSets[testDir] = fileList
		}
	}
}

func testDataSet(endpoint string, testSet string, resultChan chan bool) {


	workingDir, err := ioutil.TempDir(".", "")
	if  err != nil {
		fmt.Printf("Couldn't create directory for %s\n", workingDir)
	}
	defer os.RemoveAll(workingDir)

	curDir, err := os.Getwd()
	if  err != nil {
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
	for _, remoteFile := range testSets[testSet] {
		// Setup context to terminate commands after 600 seconds

		origURI := "root://" + endpoint + "/" + remoteFile
		cmd := exec.CommandContext(ctx, "xrdcp", origURI, ".")

		cmd.Stdout = &out
		cmd.Env = append(os.Environ(),
			"XRD_REQUESTTIMEOUT=30",   // Wait 30s before timing out
			"XRD_CPCHUNKSIZE=8388608", // read 8MB at a time
			"XRD_TIMEOUTRESOLUTION=5", // Check for timeouts every 5s
			"XRD_CONNECTIONWINDOW=30", // Wait 30s for initial TCP connection
			"XRD_CONNECTIONRETRY=2",   // Retry 2 times
			"XRD_STREAMTIMEOUT=30") // Wait 30s for TCP activity


		if err := cmd.Run(); err != nil {
			fmt.Printf("Can't download %s\nError: %s\n", origURI, err)
			resultChan <- false
		}
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

func testEndpoint(endpoint string, c chan bool) {
	workDir, err := ioutil.TempDir("", "")
	testsSucceeded := true
	if err != nil {
		fmt.Println("Couldn't create test directory for ", endpoint, ": ", err)
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
	for _, testSet := range testSetDirs {
		go testDataSet(endpoint, testSet, testResultChan)
		success := <-testResultChan
		testsSucceeded = testsSucceeded && success
		if !success {
			fmt.Printf("Failed to verify %s using endpoint %s\n", testSet, endpoint)
		}
	}

	if os.Chdir(curDir) != nil {
		c <- false
		return
	}
	c <- testsSucceeded
}

func main() {

	c := make(chan bool)
	populateTestSets(testSets)
	for _, endpoint := range endpoints {
		go testEndpoint(endpoint, c)
		success := <-c
		if !success {
			fmt.Printf("%s failed testing\n", endpoint)
		} else {
			fmt.Printf("%s passed testing\n", endpoint)
		}
	}

}
