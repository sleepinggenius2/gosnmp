// Copyright 2012-2018 The GoSNMP Authors. All rights reserved.  Use of this
// source code is governed by a BSD-style license that can be found in the
// LICENSE file.

package gosnmp

import (
	"fmt"

	"github.com/sleepinggenius2/gosmi/types"
)

func (x *GoSNMP) walk(getRequestType PDUType, rootOid types.Oid, walkFn WalkFunc) error {
	if len(rootOid) == 0 {
		rootOid = baseOid
	}

	oid := rootOid
	requests := 0
	maxReps := x.MaxRepetitions
	if maxReps == 0 {
		maxReps = defaultMaxRepetitions
	}

	// AppOpt 'c: do not check returned OIDs are increasing'
	checkIncreasing := true
	if x.AppOpts != nil {
		if _, ok := x.AppOpts["c"]; ok {
			if getRequestType == GetBulkRequest || getRequestType == GetNextRequest {
				checkIncreasing = false
			}
		}
	}

RequestLoop:
	for {

		requests++

		var response *SnmpPacket
		var err error

		switch getRequestType {
		case GetBulkRequest:
			response, err = x.GetBulkOID([]types.Oid{oid}, uint8(x.NonRepeaters), uint8(maxReps))
		case GetNextRequest:
			response, err = x.GetNextOID([]types.Oid{oid})
		case GetRequest:
			response, err = x.GetOID([]types.Oid{oid})
		default:
			response, err = nil, fmt.Errorf("Unsupported request type: %d", getRequestType)
		}

		if err != nil {
			return err
		}
		if len(response.Variables) == 0 {
			break RequestLoop
		}

		if response.Error == NoSuchName {
			x.Logger.Print("Walk terminated with NoSuchName")
			break RequestLoop
		}

		for i, pdu := range response.Variables {
			if pdu.Type == EndOfMibView || pdu.Type == NoSuchObject || pdu.Type == NoSuchInstance {
				x.Logger.Printf("BulkWalk terminated with type 0x%x", pdu.Type)
				break RequestLoop
			}
			if !pdu.Oid.ChildOf(rootOid) {
				// Not in the requested root range.
				// if this is the first request, and the first variable in that request
				// and this condition is triggered - the first result is out of range
				// need to perform a regular get request
				// this request has been too narrowly defined to be found with a getNext
				// Issue #78 #93
				if requests == 1 && i == 0 {
					getRequestType = GetRequest
					continue RequestLoop
				}
				break RequestLoop
			}

			if checkIncreasing && !pdu.Oid.After(oid) {
				return fmt.Errorf("OID not increasing: %s", pdu.Oid)
			}

			// Report our pdu
			if err := walkFn(pdu); err != nil {
				return err
			}
		}
		// Save last oid for next request
		oid = response.Variables[len(response.Variables)-1].Oid

	}
	x.Logger.Printf("BulkWalk completed in %d requests", requests)
	return nil
}

func (x *GoSNMP) walkAll(getRequestType PDUType, rootOid types.Oid) (results []SnmpPDU, err error) {
	err = x.walk(getRequestType, rootOid, func(dataUnit SnmpPDU) error {
		results = append(results, dataUnit)
		return nil
	})
	return results, err
}
