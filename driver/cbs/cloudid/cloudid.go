/*
 * Tencent is pleased to support the open source community by making TKE
 * available.
 *
 * Copyright (C) 2018 THL A29 Limited, a Tencent company. All rights reserved.
 *
 * Licensed under the BSD 3-Clause License (the "License"); you may not use this
 * file except in compliance with the License. You may obtain a copy of the
 * License at
 *
 * https://opensource.org/licenses/BSD-3-Clause
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
 * License for the specific language governing permissions and limitations under
 * the License.
 */

package cloudid

import (
	"regexp"
)

var reg = regexp.MustCompile(".*-")

const (
	Divisor = 100
)

type IDResource struct {
	prefix        string
	password      string
	regionWrapper uint64
}

// 托管CXM 0, 托管CVM 1, 超级节点 2, IDC节点 3
var (
	// set all fields to empty, in order to throws an error when used it's method.
	Invalid = IDResource{
		prefix:        "",
		password:      "",
		regionWrapper: 0,
	}

	MachineSet = IDResource{
		prefix:        "np",
		password:      "tke-np",
		regionWrapper: 0 * Divisor,
	}

	CXM = IDResource{
		prefix:        "eks",
		password:      "tke-eks",
		regionWrapper: 0 * Divisor,
	}

	CVM = IDResource{
		prefix:        "ins",
		password:      "tke-ins",
		regionWrapper: 1 * Divisor,
	}
)

// id resource classification, used for different purpose
var (
	KubernetesNodes = []IDResource{CXM, CVM}
	All             = append(KubernetesNodes, MachineSet, Invalid)
)

func (ir IDResource) EncodeID(seqID uint64, regionID uint) string {
	return EncodeID(seqID, ir.prefix, ir.password, regionID+uint(ir.regionWrapper), 8)
}

func (ir IDResource) DecodeID(id string) (uint64, uint64, error) {
	seqID, wrappedRegionID, err := DecodeID(id, ir.password)
	if err != nil {
		return 0, 0, err
	}

	return seqID, wrappedRegionID - uint64(ir.regionWrapper), nil
}

func (ir IDResource) Prefix() string {
	return ir.prefix
}

func DecodeIDWithType(ir IDResource, id string) (uint64, uint64, error) {
	return ir.DecodeID(id)
}

// input: eks-fq1u73be
// output: kn-fq1u73be
func ToKnID(id string) string {
	return reg.ReplaceAllString(id, "kn-")
}

// NOTICE: make sure the input is a knID
// input: kn-fq1u73be
// output: eks-fq1u73be
func FromKnID(knID string) string {
	ir := IDResourceFromID(knID, KubernetesNodes...)
	return reg.ReplaceAllString(knID, ir.prefix+"-")
}

// The same as IDResourceFromID(knID), but this function has
// a better performance because with the category specified
func IDResourceFromKnID(knID string) IDResource {
	return IDResourceFromID(knID, KubernetesNodes...)
}

func IDResourceFromID(id string, irs ...IDResource) IDResource {
	if len(irs) == 0 {
		irs = All
	}

	for _, ir := range irs {
		_, regionID, err := DecodeID(id, ir.password)
		if err == nil && (regionID > 0 && regionID <= 511) && (regionID/Divisor == ir.regionWrapper/Divisor) {
			return ir
		}
	}

	return Invalid
}
