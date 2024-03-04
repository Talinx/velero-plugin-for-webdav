/*
Copyright 2024 Christoph Raitzig

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

package plugin

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/studio-b12/gowebdav"
)

type WebDAVObjectStore struct {
	log        logrus.FieldLogger
	root       string
	user       string
	password   string
	bucketsDir string // bucketsDir ends in / or is empty
	logLevel   string
}

func NewWebDAVObjectStore(log logrus.FieldLogger) *WebDAVObjectStore {
	return &WebDAVObjectStore{log: log}
}

func (w WebDAVObjectStore) PrintInfos() bool {
	return w.logLevel == "INFO" || w.logLevel == "DEBUG"
}

func (w WebDAVObjectStore) PrintWarnings() bool {
	return w.logLevel == "" || w.logLevel == "WARN" || w.logLevel == "INFO" || w.logLevel == "DEBUG"
}

func (w *WebDAVObjectStore) Init(config map[string]string) error {
	root := config["root"]
	user := config["user"]
	password := config["password"]
	bucketsDir := config["bucketsDir"]
	logLevel := config["logLevel"]
	bucket := config["bucket"]
	prefix := config["prefix"]
	w.root = root
	w.user = user
	w.password = password
	if bucketsDir != "" && !strings.HasPrefix(bucketsDir, "/") {
		bucketsDir = fmt.Sprintf("%s/", bucketsDir)
	}
	w.bucketsDir = bucketsDir
	w.logLevel = strings.ToUpper(logLevel)

	if root == "" {
		w.log.Errorf("WebDAV root is empty - please provide a valid URL")
	}
	if user == "" {
		w.log.Errorf("WebDAV username is empty")
	}
	if password == "" && w.PrintWarnings() {
		w.log.Warnf("WebDAV password is empty")
	}
	if root != "" && user != "" && password != "" && w.PrintInfos() {
		w.log.Infof("Server root, username and password for WebDAV are all set")
	}
	if w.PrintInfos() {
		w.log.Infof("Using bucket '%s' with path prefix '%s'", bucket, prefix)
	}

	return nil
}

func SplitPathToDirAndFilename(path string) (dir string, name string) {
	lastSeparatorI := strings.LastIndex(path, "/")
	dir, name = "", path
	if lastSeparatorI != -1 {
		dir, name = path[:lastSeparatorI], path[lastSeparatorI+1:]
	}
	return dir, name
}

func (w *WebDAVObjectStore) PutObject(bucket string, key string, body io.Reader) error {
	path := fmt.Sprintf("%s%s/%s", w.bucketsDir, bucket, key)
	dir, _ := SplitPathToDirAndFilename(path)

	c := gowebdav.NewClient(w.root, w.user, w.password)
	err := c.Connect()
	if err != nil {
		w.log.Errorf("Error connecting to WebDAV server")
		w.log.WithError(err)
		return err
	}

	err = c.MkdirAll(dir, 0755)
	if err != nil {
		return err
	}

	return c.WriteStream(path, body, 0755)
}

func (w *WebDAVObjectStore) ObjectExists(bucket, key string) (bool, error) {
	path := fmt.Sprintf("%s%s/%s", w.bucketsDir, bucket, key)
	dir, name := SplitPathToDirAndFilename(path)

	c := gowebdav.NewClient(w.root, w.user, w.password)
	err := c.Connect()
	if err != nil {
		w.log.Errorf("Error connecting to WebDAV server")
		w.log.WithError(err)
		return false, err
	}

	// last separator is usually between bucket and key
	// search for it anyway to make the function more generic in case the delimiter is not "/"
	// lastSeparatorI := strings.LastIndex(path, "/")
	// dir, name := "", path
	// if lastSeparatorI != -1 {
	// 	dir, name = path[:lastSeparatorI], path[lastSeparatorI+1:]
	// }

	files, err := c.ReadDir(dir)
	if err != nil {
		if gowebdav.IsErrNotFound(err) {
			return false, nil
		}
		return false, err
	}
	for _, file := range files {
		if !file.IsDir() && file.Name() == name {
			return true, nil
		}
	}
	return false, nil
}

func (w *WebDAVObjectStore) GetObject(bucket, key string) (io.ReadCloser, error) {
	path := fmt.Sprintf("%s%s/%s", w.bucketsDir, bucket, key)

	c := gowebdav.NewClient(w.root, w.user, w.password)
	err := c.Connect()
	if err != nil {
		w.log.Errorf("Error connecting to WebDAV server")
		w.log.WithError(err)
		return nil, err
	}

	return c.ReadStream(path)
}

func AddDirsWithCommonPrefixes(w *WebDAVObjectStore, c *gowebdav.Client, accumulatedDirs []string, inputDirs []os.FileInfo, completePrefix string, prefixToCut string, parentDirName string) ([]string, bool, error) {
	outputAccumulatedDirs := accumulatedDirs
	allFilesDirs := true
	var allSubfilesDirs bool
	for _, currentFile := range inputDirs {
		completePath := fmt.Sprintf("%s%s/", parentDirName, currentFile.Name())
		if !strings.HasPrefix(completePath, completePrefix) {
			continue
		}
		commonPrefix, found := strings.CutPrefix(completePath, prefixToCut)
		if !found {
			continue
		}
		if currentFile.IsDir() {
			subDirs, err := c.ReadDir(completePath)
			if err != nil {
				return outputAccumulatedDirs, allFilesDirs, err
			}
			outputAccumulatedDirs, allSubfilesDirs, err = AddDirsWithCommonPrefixes(w, c, outputAccumulatedDirs, subDirs, completePrefix, prefixToCut, completePath)
			if err != nil {
				return outputAccumulatedDirs, allFilesDirs, err
			}
			if !allSubfilesDirs {
				// only add directory if it contains at least one file (that is not a directory)
				outputAccumulatedDirs = append(outputAccumulatedDirs, commonPrefix)
			}
		} else {
			allFilesDirs = false
		}
	}
	return outputAccumulatedDirs, allFilesDirs, nil
}

func (w *WebDAVObjectStore) ListCommonPrefixes(bucket, prefix, delimiter string) ([]string, error) {
	/* List all folders in the directory named by the bucket and prefix parameters.
	For example, if bucket = "backups" and prefix = "my-app" and the directory structure is
	backups/
	├── my-app
	│   ├── cars
	│   └── trains
	└── some-other-app
	    └── bridges
	Then this function would return ["my-app/cars/", "my-app/trains/"].
	Note that the bucket name "backups/" is removed from the output list. This is for compatibility with
	other cloud offerings that support buckets natively (like AWS S3).
	"some-other-app/bridges/" is also missing as it does not use the specified prefix "my-app".
	*/

	// prefix has to either be the empty string or end with /
	// fix this if not already the case
	prefixToUse := prefix
	if prefixToUse != "" && !strings.HasSuffix(prefix, "/") {
		prefixToUse = fmt.Sprintf("%s/", prefix)
	}
	prefixToCut := fmt.Sprintf("%s%s/", w.bucketsDir, bucket)

	var rootDir string

	if delimiter == "/" {
		// delimiter is equivalent to the directory separator
		// list all subfolders in bucket/prefix/
		rootDir = fmt.Sprintf("%s%s/%s", w.bucketsDir, bucket, prefixToUse)
		// rootDir ends with /
	} else {
		// Because the delimiter is not the directory separator we have to consider all folders in the bucket
		rootDir = bucket
	}

	var dirs []string

	c := gowebdav.NewClient(w.root, w.user, w.password)
	err := c.Connect()
	if err != nil {
		w.log.Errorf("Error connecting to WebDAV server")
		w.log.WithError(err)
		return dirs, err
	}

	rootSubdirs, err := c.ReadDir(rootDir)
	if err != nil {
		if gowebdav.IsErrNotFound(err) {
			// root directory does not currently exists
			// this is okay, we only create a directory when we first put a file in it
			return dirs, nil
		} else {
			w.log.Errorf("Error reading directory '%s' via WebDAV", rootDir)
			w.log.WithError(err)
			return dirs, err
		}
	}

	if delimiter == "/" {
		// traverse into all subdirectories
		// if prefix != "" {
		// 	// TODO: remove bucket name
		// 	dirs = append(dirs, rootDir)
		// }
		dirs, _, err = AddDirsWithCommonPrefixes(w, c, dirs, rootSubdirs, rootDir, prefixToCut, rootDir)
		if err != nil {
			w.log.Errorf("Got error reading directories via WebDAV")
			w.log.WithError(err)
		}
	} else {
		// TODO
		// get all directory names and only return those matching the prefix
	}

	return dirs, nil
}

func (w *WebDAVObjectStore) ListObjects(bucket, prefix string) ([]string, error) {
	prefixToUse := prefix
	if prefixToUse != "" && !strings.HasSuffix(prefix, "/") {
		prefixToUse = fmt.Sprintf("%s/", prefix)
	}
	path := fmt.Sprintf("%s%s/%s", w.bucketsDir, bucket, prefixToUse)

	var objects []string

	c := gowebdav.NewClient(w.root, w.user, w.password)
	err := c.Connect()
	if err != nil {
		w.log.Errorf("Error connecting to WebDAV server")
		w.log.WithError(err)
		return objects, err
	}

	files, err := c.ReadDir(path)
	if err != nil {
		if gowebdav.IsErrNotFound(err) {
			return objects, nil
		} else {
			w.log.Errorf("Error reading directory '%s' via WebDAV", path)
			w.log.WithError(err)
			return objects, err
		}
	}

	prefixToCut := fmt.Sprintf("%s%s/", w.bucketsDir, bucket)
	for _, file := range files {
		if !file.IsDir() {
			completePath := fmt.Sprintf("%s%s/", path, file.Name())
			filenameWithoutBucket, found := strings.CutPrefix(completePath, prefixToCut)
			if !found {
				continue
			}
			objects = append(objects, filenameWithoutBucket)
		}
	}
	return objects, nil
}

func (w *WebDAVObjectStore) DeleteObject(bucket, key string) error {
	path := fmt.Sprintf("%s%s/%s", w.bucketsDir, bucket, key)
	dir, _ := SplitPathToDirAndFilename(path)

	c := gowebdav.NewClient(w.root, w.user, w.password)
	err := c.Connect()
	if err != nil {
		w.log.Errorf("Error connecting to WebDAV server")
		w.log.WithError(err)
		return err
	}

	err = c.Remove(path)
	if err != nil {
		return err
	}

	files, err := c.ReadDir(dir)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		err := c.Remove(dir)
		if err != nil {
			return err
		}
	}

	return nil
}

func (w *WebDAVObjectStore) CreateSignedURL(bucket, key string, ttl time.Duration) (string, error) {
	return "", errors.New("CreateSignedURL is not supported for this plugin")
}
