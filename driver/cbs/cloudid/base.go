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
	"bytes"
	"fmt"
	"strings"
)

var regionID uint

func Init(id uint) {
	regionID = id
}

// 将整型转换成字符串
// 输入
//  v 整型
//  base 转换的进制
//  buf 保存返回的字符串
//  len buf的长度
// 输出
//  无

/**static void int_2_str(uint64_t v, unsigned char base, char * buf, unsigned char len)
**/

func int2Str(v uint64, base uint8, buf []uint8, len uint8) {
	var m uint8 = 0
	var i uint8 = 0
	for {
		if i >= len-1 || v <= 0 {
			break
		}
		m = uint8(v % uint64(base))
		i++
		if m <= 9 {
			buf[len-i-1] = '0' + m
		} else {
			buf[len-i-1] = 'a' + m - 10
		}
		v = v / uint64(base)
	}
	buf[len-1] = 0
	for {
		if i >= len-1 {
			break
		}
		i++
		buf[len-i-1] = '0'
	}
}

// 求幂
// 输入
//  x 幂底
//  y 幂倍
// 输出
//  返回幂
// static uint64_t iPow(unsigned char x, unsigned char y)
func iPow(x, y uint8) uint64 {
	var ret uint64 = 1
	var i uint8 = 0
	for i = 0; i < y; i++ {
		ret = ret * uint64(x)
	}
	return ret
}

// 将字符串转换成整型
// 输入
//  buf 字符串
//  len 字符串长度
//  base 转换进制
// 输出
//  返回整型
// static uint64_t str_2_int(const char * buf, unsigned char len, unsigned char base)
func str2Int(buf []byte, len uint8, base uint8) uint64 {
	var ret uint64 = 0
	var m, i uint8

	for i = 0; i < len-1; i++ {
		m = buf[len-2-i]
		if m >= '0' && m <= '9' {
			m = m - '0'
		} else {
			m = m - 'a' + 10
		}
		ret += iPow(base, i) * uint64(m)
	}
	return ret
}

// 将加密字符串转换成整型
// 输入
//  buf 字符串
//  len 字符串长度
//  retLen 保留bit位
// 输出
//  返回转换后的整型
// static uint64_t encode_password(const char * buf, unsigned char len, unsigned char retLen)
func encodePassword(buf []byte, len uint8, retLen uint8) uint64 {
	var ret uint64 = 0
	var i uint8 = 0
	for i = 0; i < len-1; i++ {
		ret |= uint64(buf[len-2-i]) << (8 * i)
	}
	if retLen > 0 {
		ret &= ((uint64)(1) << retLen) - 1
	}
	return ret
}

// 根据整型得到加密字符串
// 输入
//  v 整型
//  buf 保存返回的字符串
//  len buf的长度
// 输出
//  无
// static void decode_password(uint64_t v, char * buf, unsigned char len)
func decodePassword(v uint64, buf []byte, len uint8) {
	var i uint8 = 1
	for {
		if i >= len {
			break
		}
		if v <= 0 {
			break
		}
		buf[len-i-1] = byte(v & 255)
		i++
		v >>= 8
	}
	buf[len-1] = 0
}

type dxy struct {
	d int64
	x int64
	y int64
}

type pair struct {
	x uint64 //id
	y uint64 //regionID
}

// 参考算法导论 欧几里德算法的推广形式
// d = ax + by
// 根据欧几里德算法推广形式计算出d,x,y
// 输入
//  a,b
// 输出
//  d,x,y
// static struct dxy extended_euclid(int64_t a, int64_t b)
func extendedEuclid(a, b int64) dxy {
	if b <= 0 {
		ret := dxy{a, 1, 0}
		return ret
	}
	tmp := extendedEuclid(b, a%b)
	ret := dxy{tmp.d, tmp.y, tmp.x - (a/b)*tmp.y}
	return ret
}

// 参考算法导论 推论31.26
// 当gcd(a, b) = 1, 则方程ax + ny = 1有唯一解,这个唯一解就是
// extended_euclid(a, b)算法得到的x
// 输入
//  a,b
// 输出
//  x
// static int64_t inverse(int64_t a, int64_t b)
func inverse(a, b int64) int64 {
	tmp := extendedEuclid(a, b)
	if tmp.x < 0 {
		tmp.x = tmp.x%b + b
	}
	return tmp.x
}

// 获取val从pos位置长度为len的bit
// 输入
//  val 整型
//  pos 起始位置
//  len 长度
// 输出
//  返回截取的bit
/**static uint64_t split_bit(uint64_t val, unsigned char pos, unsigned char len)
**/
func splitBit(val uint64, pos uint8, len uint8) uint64 {
	return (val >> pos) & ((1 << len) - 1)
}

// 返回postfix插入prefix的平均步长
// 输入
//  prefixLen 前缀长度
//  postfixLen 后缀长度
// 输出
//  回postfix插入prefix的平均步长
/**static unsigned char step_len(unsigned char prefixLen, unsigned char postfixLen)
**/

func stepLen(prefixLen, postfixLen uint8) uint8 {
	return prefixLen / postfixLen
}

// 回postfix入prefix的步长
// 输入
//  step_idx 步骤游标
//  step_len 平均步长
//  prefixLen 前缀长度
//  postfixLen 后缀长度
// 输出
//  返回postfix插入prefix的步长
/**static unsigned char step_postfixLen(unsigned char step_idx, unsigned char step_len,
        unsigned char prefixLen, unsigned char postfixLen)
**/
func stepPostfixLen(stepIdx, stepLen, prefixLen, postfixLen uint8) uint8 {
	if stepIdx == (postfixLen - 1) {
		return prefixLen - (stepIdx * stepLen)
	} else {
		return stepLen
	}
}

// 将postfix打散到prefix中
// 输入
//  prefix 前缀
//  prefixLen 前缀长度
//  postfix 后缀
//  postfixLen 后缀长度
// 输出
//  返回打散的整型
/**static uint64_t shuffle_prefix_postfix(uint64_t prefix, unsigned char prefixLen,
        uint64_t postfix, unsigned char postfixLen)
**/
func shufflePrefixPostfix(prefix uint64, prefixLen uint8,
	postfix uint64, postfixLen uint8) uint64 {
	var ret uint64 = 0
	var i uint8 = 0

	step := stepLen(prefixLen, postfixLen)
	for i = 0; i < postfixLen; i++ {
		ret |= ((splitBit(prefix, i*step, stepPostfixLen(i, step, prefixLen, postfixLen)) << 1) | splitBit(postfix, i, 1)) << ((step + 1) * i)
	}
	return ret
}

// 取出postfix以及prefix
// 输入
//  val 打散后的整型
//  prefixLen 前缀长度
//  postfixLen 后缀长度
// 输出
// 返回前缀、后缀
/**static pair split_prefix_postfix(uint64_t val, unsigned char prefixLen,
        unsigned char postfixLen)
**/
func splitPrefixPostfix(val uint64, prefixLen uint8,
	postfixLen uint8) pair {
	var prefix uint64 = 0
	var postfix uint64 = 0
	var i uint8 = 0
	step := stepLen(prefixLen, postfixLen)
	for i = 0; i < postfixLen; i++ {
		postfix |= splitBit(val, (step+1)*i, 1) << i
		prefix |= splitBit(val, (step+1)*i+1, stepPostfixLen(i, step, prefixLen, postfixLen)) << (step * i)
	}
	ret := pair{prefix, postfix}
	return ret
}

// 加密得到ID
// 输入
//  prefix 前缀
//  prefixLen 前缀长度
//  postfix 后缀
//  postfixLen 后缀长度
//  password 加密字符串
//  base 转换进制
//  prime 素数
//  buf 保存返回的字符串ID
//  retLen 返回的字符串长度
// 输出
//  无
/**static void encode(uint64_t prefix, unsigned char prefixLen, uint64_t postfix, unsigned char postfixLen,
        const char * password, unsigned char base, uint64_t prime, char * buf, unsigned char len)
**/
func encode(prefix uint64, prefixLen uint8, postfix uint64, postfixLen uint8,
	password []byte, base uint8, prime uint64, buf []byte, bufLen uint8) {
	iPassword := encodePassword(password, uint8(len(password)+1), prefixLen+postfixLen)
	ret := shufflePrefixPostfix(uint64(inverse(int64(prefix), int64(prime))), prefixLen, postfix, postfixLen)
	ret ^= iPassword
	int2Str(ret, base, buf, bufLen)
}

// 从字符串ID得到前缀、后缀
// 输入
//  buf 字符串ID
//  prefixLen 前缀长度
//  postfixLen 后缀长度
//  password 加密字符串
//  base 转换进制
//  prime 素数
// 输出
//  返回前缀、后缀
/**static pair decode(const char * buf, unsigned char prefixLen, unsigned char postfixLen,
        const char * password, unsigned char base, uint64_t prime)
**/

func Decode(buf []byte, prefixLen uint8, postfixLen uint8,
	password []byte, base uint8, prime uint64) pair {
	iPassword := encodePassword(password, uint8(len(password)+1), prefixLen+postfixLen)
	ret := str2Int(buf, uint8(len(buf)+1), base)
	ret ^= iPassword

	pRet := splitPrefixPostfix(ret, prefixLen, postfixLen)
	pRet.x = uint64(inverse(int64(pRet.x), int64(prime)))
	return pRet
}

// 前缀长度映射信息
var prefixLenMap = [3]uint8{32, 36, 40}

//static const unsigned char prefixLenMap[] = {32, 36, 40};

// 后缀长度映射信息
//static const unsigned char postfixLenMap[] = {9, 10, 11};
var postfixLenMap = [3]uint8{9, 10, 11}

// 素数映射信息
//static const uint64_t primeMap[] = {4294967029, 68719476503, 1000000005721};
var primeMap = [3]uint64{4294967029, 68719476503, 1000000005721}

// 转换进制
//static unsigned char idBase = 36;
var idBase uint8 = 36

// ID分隔符
//static const char idSEPARATOR = '-';
var idSEPARATOR byte = '-'

// 将整型唯一ID转换成字符串ID
// 输入
//  obj_id 对象分地域唯一性ID
//  abbreviate 对象缩写
//  password 对象加密字符串 长度5-8位
//  zone 地域信息
//  idLen 返回的ID长度(不包括缩写) 8-10
//  buf 保存返回的字符串ID
//  len buf的长度
// 输出
//  转换是否成功 -1 失败 0 成功
/**int encode_id(uint64_t obj_id, const char * abbreviate, const char * password,
        uint64_t zone, unsigned char idLen, char * buf, unsigned char len)
**/
// 8 <= idLen <= 10
func EncodeID(id uint64, abbreviate string, password string, regionID uint, idLen uint) string {
	if idLen < 8 || idLen > 10 {
		return ""
	}
	var prefixLen = prefixLenMap[idLen-8]
	var postfixLen = postfixLenMap[idLen-8]
	var prime = primeMap[idLen-8]

	var idBuf []byte
	idBuf = make([]byte, idLen+1, idLen+1)

	encode(id, prefixLen, uint64(regionID), postfixLen,
		[]byte(password), idBase, prime, idBuf, uint8(len(idBuf)))

	var buf []byte
	buf = make([]byte, 32, 32)

	var abbreviateLen = len([]byte(abbreviate))
	if abbreviateLen+1+len(idBuf) >= 32 {
		return ""
	}
	copy(buf, []byte(abbreviate))
	buf[abbreviateLen] = idSEPARATOR

	copy(buf[abbreviateLen+1:], idBuf)
	return string(bytes.Trim(buf, "\x00"))

}

// 8 <= idLen <= 10
/**func encodeID(id uint64, abbreviate string, password string, regionID uint, idLen uint) string {
**/

// 从字符串ID分解出整型ID以及地域信息
// 输入
//  buf 字符串ID
//  password 象加密字符串
//  ret 保存整型ID以及地域
// 输出
//  转换是否成功 -1 失败 0 成功
/**int decode_id(const char * buf, const char * password, pair * ret)
**/

//func DecodeId(buf []byte, password []byte, ret *pair) int {
func DecodeID(uID string, password string) (x uint64, y uint64, err error) {
	p := pair{}

	index := strings.IndexByte(uID, idSEPARATOR)
	if index < 0 {
		return 0, 0, fmt.Errorf("failed to decode id:%s, ret:%v", uID, -1)
	}
	buf := []byte(uID)

	idBuf := buf[index+1:]
	var idLen = len(idBuf)
	if idLen < 8 || idLen > 10 {
		return 0, 0, fmt.Errorf("id %s len after - should between 8 and 10", uID)
	}
	var prefixLen = prefixLenMap[idLen-8]
	var postfixLen = postfixLenMap[idLen-8]
	var prime = primeMap[idLen-8]
	p = Decode(idBuf, prefixLen, postfixLen, []byte(password), idBase, prime)
	return p.x, p.y, nil
}
