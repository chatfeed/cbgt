//  Copyright (c) 2014 Couchbase, Inc.
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the
//  License. You may obtain a copy of the License at
//    http://www.apache.org/licenses/LICENSE-2.0
//  Unless required by applicable law or agreed to in writing,
//  software distributed under the License is distributed on an "AS
//  IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
//  express or implied. See the License for the specific language
//  governing permissions and limitations under the License.

package cbgt

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/couchbase/cbauth"
)

// ----------------------------------------------------------------

var ErrCouchbaseMismatchedBucketUUID = fmt.Errorf("mismatched-couchbase-bucket-UUID")

// Frequency of type time.Duration to check the state of the cluster
// that the couchbase.Bucket instance is a part of.
var CouchbaseNodesRecheckInterval = 5 * time.Second

// ----------------------------------------------------------------

// ParsePartitionsToVBucketIds is specific to couchbase
// data-sources/feeds, converting a set of partition strings from a
// dests map to vbucketId numbers.
func ParsePartitionsToVBucketIds(dests map[string]Dest) ([]uint16, error) {
	vbuckets := make([]uint16, 0, len(dests))
	for partition := range dests {
		if partition != "" {
			vbId, err := strconv.Atoi(partition)
			if err != nil {
				return nil, fmt.Errorf("feed_cb:"+
					" could not parse partition: %s, err: %v", partition, err)
			}
			vbuckets = append(vbuckets, uint16(vbId))
		}
	}
	return vbuckets, nil
}

// VBucketIdToPartitionDest is specific to couchbase
// data-sources/feeds, choosing the right Dest based on a vbucketId.
func VBucketIdToPartitionDest(pf DestPartitionFunc,
	dests map[string]Dest, vbucketId uint16, key []byte) (
	partition string, dest Dest, err error) {
	if vbucketId < uint16(len(vbucketIdStrings)) {
		partition = vbucketIdStrings[vbucketId]
	}
	if partition == "" {
		partition = fmt.Sprintf("%d", vbucketId)
	}
	dest, err = pf(partition, key, dests)
	if err != nil {
		return "", nil, fmt.Errorf("feed_cb: VBucketIdToPartitionDest,"+
			" partition func, vbucketId: %d, err: %v", vbucketId, err)
	}
	return partition, dest, err
}

// vbucketIdStrings is a memoized array of 1024 entries for fast
// conversion of vbucketId's to partition strings via an index lookup.
var vbucketIdStrings []string

func init() {
	vbucketIdStrings = make([]string, 1024)
	for i := 0; i < len(vbucketIdStrings); i++ {
		vbucketIdStrings[i] = fmt.Sprintf("%d", i)
	}
}

// ----------------------------------------------------------------

// CouchbaseParseSourceName parses a sourceName, if it's a couchbase
// REST/HTTP URL, into a server URL, poolName and bucketName.
// Otherwise, returns the serverURLDefault, poolNameDefault, and treat
// the sourceName as a bucketName.
func CouchbaseParseSourceName(
	serverURLDefault, poolNameDefault, sourceName string) (
	string, string, string) {
	if !strings.HasPrefix(sourceName, "http://") &&
		!strings.HasPrefix(sourceName, "https://") {
		return serverURLDefault, poolNameDefault, sourceName
	}

	u, err := url.Parse(sourceName)
	if err != nil {
		return serverURLDefault, poolNameDefault, sourceName
	}

	a := strings.Split(u.Path, "/")
	if len(a) != 5 ||
		a[0] != "" ||
		a[1] != "pools" ||
		a[2] == "" ||
		a[3] != "buckets" ||
		a[4] == "" {
		return serverURLDefault, poolNameDefault, sourceName
	}

	v := url.URL{
		Scheme: u.Scheme,
		User:   u.User,
		Host:   u.Host,
	}

	server := v.String()
	poolName := a[2]
	bucketName := a[4]

	return server, poolName, bucketName
}

// ----------------------------------------------------------------

// CBAuthHttpGet is a couchbase-specific http.Get(), for use in a
// cbauth'ed environment.
func CBAuthHttpGet(urlStrIn string) (resp *http.Response, err error) {
	urlStr, err := CBAuthURL(urlStrIn)
	if err != nil {
		return nil, err
	}

	return http.Get(urlStr)
}

// CBAuthURL rewrites a URL with credentials, for use in a cbauth'ed
// environment.
func CBAuthURL(urlStr string) (string, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}

	cbUser, cbPasswd, err := cbauth.GetHTTPServiceAuth(u.Host)
	if err != nil {
		return "", err
	}

	u.User = url.UserPassword(cbUser, cbPasswd)

	return u.String(), nil
}

// ----------------------------------------------------------------

func parseParams(src string,
	req *http.Request) (string, string, string, error) {
	// Split the provided src on ";" as the user is permitted
	// to provide multiple servers(urls) concatenated with a ";".
	servers := strings.Split(src, ";")

	var u *url.URL
	var err error
	for _, server := range servers {
		u, err = url.Parse(server)
		if err == nil {
			break
		}
	}

	if err != nil {
		return "", "", "", err
	}
	v := url.URL{
		Scheme: u.Scheme,
		User:   u.User,
		Host:   u.Host,
	}
	uname, pwd, err := cbauth.ExtractCreds(req)
	if err != nil {
		return "", "", "", err
	}
	return v.String(), uname, pwd, nil
}
