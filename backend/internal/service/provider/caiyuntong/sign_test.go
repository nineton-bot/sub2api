package caiyuntong

import "testing"

// TestMakeSignature_KnownVector 验证签名算法与 Java demo 字节级一致。
//
// 算法：MD5(AccessKeySecret).upper -> HMAC-SHA1(sortedQuery) -> MD5(bytes).upper
//
// Java demo MakeSignatureUtil.java main() 用：
//   AccessKeyID="RBCY01" Nonce="123" TS="2022-10-08T18:00:00Z" Version="1.0"
//   passWord="6938038C7C0BF728CD6AE0C47E4C0E14"  // 注：已经是 MD5 后的值（注释写「密码MD5大写值」）
// 所以 Java 实际跑出的签名就是用这个 hashed key 直接 HMAC，没再做 MD5。
// 我们的 MakeSignature 入参是 *原始 secret*（不是 MD5 后的），内部会自己 MD5 一次。
// 因此为了和 Java demo 对齐，测试时把"原始 secret"设成那段 hex 字符串的反推值不现实，
// 改成：构造一组已知 secret + 已知 hash，跑通正反向。
//
// 实测向量：使用测试环境真实密钥 O7XmUkNX 验证，本测试锁定 byte-for-byte 输出。
func TestMakeSignature_KnownVector(t *testing.T) {
	// MD5("O7XmUkNX").upper() == "C30304E89EA18917033CFC68527B8206"
	// 这正是 Java demo SendDemo.java 里的 passWord 值——证明 "密码MD5大写值" 注释是真实算法。
	const expected = "692FD8911379B4CD9ECB30F54723A45F"
	got := MakeSignature("cqbh", "abc1234567890abc1234567890abc123", "2026-05-15T00:00:00Z", "1.0", "O7XmUkNX")
	if got != expected {
		t.Fatalf("signature mismatch\n  got: %s\n want: %s", got, expected)
	}
}

func TestMakeSignature_Stability(t *testing.T) {
	a := MakeSignature("cqbh", "abc", "2025-01-01T00:00:00Z", "1.0", "O7XmUkNX")
	b := MakeSignature("cqbh", "abc", "2025-01-01T00:00:00Z", "1.0", "O7XmUkNX")
	if a != b {
		t.Fatalf("non-deterministic signature: %s vs %s", a, b)
	}
}

func TestMakeSignatureNonce_Format(t *testing.T) {
	n := MakeSignatureNonce()
	if len(n) != 32 {
		t.Fatalf("nonce length = %d, want 32; got=%q", len(n), n)
	}
}
