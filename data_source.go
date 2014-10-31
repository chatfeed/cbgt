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

package main

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/couchbaselabs/go-couchbase"
)

type SimpleSourceParams struct {
	NumPartitions int `json:"numPartitions"`
}

func DataSourcePartitions(sourceType, sourceName,
	sourceUUID, sourceParams, server string) ([]string, error) {
	if sourceType == "couchbase" {
		poolName := "default" // TODO: Parameterize poolName.
		bucketName := sourceName

		// TODO: how the halloween does GetBucket() api work without explicit auth?
		bucket, err := couchbase.GetBucket(server, poolName, bucketName)
		if err != nil {
			return nil, fmt.Errorf("error: DataSourcePartitions/couchbase"+
				" failed GetBucket, server: %s, poolName: %s, bucketName: %s, err: %v",
				server, poolName, bucketName, err)
		}
		defer bucket.Close()

		vbm := bucket.VBServerMap()
		if vbm == nil {
			return nil, fmt.Errorf("error: DataSourcePartitions/couchbase"+
				" no VBServerMap, server: %s, poolName: %s, bucketName: %s, err: %v",
				server, poolName, bucketName, err)
		}

		// NOTE: We assume that vbucket numbers are continuous
		// integers starting from 0.
		numVBuckets := len(vbm.VBucketMap)
		rv := make([]string, numVBuckets)
		for i := 0; i < numVBuckets; i++ {
			rv[i] = strconv.Itoa(i)
		}
		return rv, nil
	}

	if sourceType == "simple" {
		ssp := &SimpleSourceParams{}
		if sourceParams != "" {
			err := json.Unmarshal([]byte(sourceParams), ssp)
			if err != nil {
				return nil, fmt.Errorf("error: DataSourcePartitions/simple"+
					" could not parse sourceParams: %s, err: %v", sourceParams, err)
			}
		}
		numPartitions := ssp.NumPartitions
		rv := make([]string, numPartitions)
		for i := 0; i < numPartitions; i++ {
			rv[i] = strconv.Itoa(i)
		}
		return rv, nil
	}

	return nil, fmt.Errorf("error: DataSourcePartitions got unknown sourceType: %s",
		sourceType)
}