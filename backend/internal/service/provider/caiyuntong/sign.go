// Package caiyuntong 实现财云通（bigfintax）销项发票接口的对接。
//
// 接口文档：销项应用系统接口 1.0.1
// 测试环境：
//
//	AccessKeyID:     cqbh
//	AccessKeySecret: O7XmUkNX
//	Endpoint:        https://api-dataservice.bigfintax.com/
//
// 签名规则参考 docx 与 demo SendDemo.java / MakeSignatureUtil.java：
//  1. 按字典序拼接 AccessKeyID / SignatureNonce / TimeStamp / Version 为 query
//  2. 以 AccessKeySecret 为 key 做 HMAC-SHA1 加密
//  3. 对密文取 MD5，输出 32 位大写十六进制
package caiyuntong

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"strings"
	"time"

	"github.com/google/uuid"
)

// SignVersion 当前接口契约版本（财云通文档要求 1.0）。
const SignVersion = "1.0"

// MakeSignature 生成 32 位大写的财云通签名。
//
// 算法（参考 Java demo SendDemo.java / MakeSignatureUtil.java）：
//  1. AccessKeySecret 先做 MD5，取 32 位大写 hex —— 这是 Java demo 里 passWord 字段的真实含义，
//     注释明确写「密码MD5大写值」。早期实现直接拿 AccessKeySecret 当 HMAC key，导致服务端
//     一直返回 "Code":"400 签名不正确"。
//  2. 用上述 hashed key 对按字典序拼接的 query string 做 HMAC-SHA1
//  3. 对 HMAC-SHA1 二进制结果再做一次 MD5，输出 32 位大写 hex
//
// 参数顺序与 Java demo 完全一致；timestamp 形如 "2006-01-02T15:04:05Z"。
func MakeSignature(accessKeyID, signatureNonce, timestamp, version, accessKeySecret string) string {
	raw := "AccessKeyID=" + accessKeyID +
		"&SignatureNonce=" + signatureNonce +
		"&TimeStamp=" + timestamp +
		"&Version=" + version

	keyMD5 := md5.Sum([]byte(accessKeySecret))
	hashedKey := strings.ToUpper(hex.EncodeToString(keyMD5[:]))

	mac := hmac.New(sha1.New, []byte(hashedKey))
	mac.Write([]byte(raw))
	hmacBytes := mac.Sum(nil)

	sum := md5.Sum(hmacBytes)
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

// MakeTimeStamp 生成签名用时间戳，对应 Java demo 的 SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss'Z'")。
//
// 注意：Java demo 用的是 system local time（在中国机器上即北京时间），格式末尾的 'Z' 是
// 字面字符（用单引号转义），并不代表 UTC。早期版本曾使用 UTC 提交，财云通服务端按北京时间
// 比对会算成 8 小时偏差，直接返回 "Code":"400 时间差异超出范围"。这里用 Asia/Shanghai
// 显式锁定时区，不依赖容器本地 TZ。
func MakeTimeStamp() string {
	return time.Now().In(time.FixedZone("CST+8", 8*3600)).Format("2006-01-02T15:04:05Z")
}

// MakeSignatureNonce 生成去横线的 UUID。
func MakeSignatureNonce() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")
}

// BuildSignedQuery 拼出完整的 query string（含 Signature 参数）。
func BuildSignedQuery(accessKeyID, accessKeySecret string) string {
	ts := MakeTimeStamp()
	nonce := MakeSignatureNonce()
	sig := MakeSignature(accessKeyID, nonce, ts, SignVersion, accessKeySecret)
	return "AccessKeyID=" + accessKeyID +
		"&SignatureNonce=" + nonce +
		"&TimeStamp=" + ts +
		"&Version=" + SignVersion +
		"&Signature=" + sig
}
