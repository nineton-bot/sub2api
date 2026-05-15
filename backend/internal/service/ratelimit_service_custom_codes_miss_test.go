//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// 覆盖账号配置 custom_error_codes 白名单但实际错误码不在列表（典型场景：白名单只
// 列了 401/429，上游返回 502）时的回落行为：
//
//   - 旧逻辑：CheckErrorPolicy / HandleUpstreamError 都直接 bail，不跑
//     temp_unschedulable 规则 → sticky session 继续粘在坏账号上，用户连续
//     重试都失败 15+ 分钟
//   - 新逻辑：白名单不命中时仍跑 temp_unschedulable，命中规则就把账号摘掉
//
// 4 条 case 镜像覆盖两个函数：
//   - rule 命中 → 提前返回（TempUnscheduled / true）
//   - rule 未命中 → 走原 Skipped / false
//   - temp_unsched 整体未开启 → 走原 Skipped / false（验证 IsTempUnschedulableEnabled 早返回）
//   - 401 守卫：custom_codes miss + 401 即使规则命中也不走 temp_unsched（HandleUpstreamError 专属）

func makeCustomCodesAccountForMissTest(rulesEnabled bool) *Account {
	creds := map[string]any{
		"custom_error_codes_enabled": true,
		"custom_error_codes":         []any{float64(401), float64(429)}, // 故意不含 502
	}
	if rulesEnabled {
		creds["temp_unschedulable_enabled"] = true
		creds["temp_unschedulable_rules"] = []any{
			map[string]any{
				"error_code":       float64(502),
				"keywords":         []any{"bad gateway"},
				"duration_minutes": float64(5),
			},
		}
	}
	return &Account{
		ID:          901,
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Credentials: creds,
	}
}

// ----- CheckErrorPolicy（修复前已加，这里把测试 guard 住防漂移） -----

func TestCheckErrorPolicy_CustomCodesMiss_TempUnschedRuleHits(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := makeCustomCodesAccountForMissTest(true)

	result := svc.CheckErrorPolicy(
		context.Background(),
		account,
		http.StatusBadGateway,
		[]byte(`{"error":"upstream bad gateway"}`),
	)

	require.Equal(t, ErrorPolicyTempUnscheduled, result,
		"custom_codes miss 时 temp_unsched 命中应升级为 TempUnscheduled，触发 failover")
	require.Equal(t, 1, repo.tempCalls, "应实际调一次 SetTempUnschedulable")
}

func TestCheckErrorPolicy_CustomCodesMiss_TempUnschedRuleMisses(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := makeCustomCodesAccountForMissTest(true)

	// body 不含 "bad gateway" 关键词，规则不命中
	result := svc.CheckErrorPolicy(
		context.Background(),
		account,
		http.StatusBadGateway,
		[]byte(`{"error":"some unrelated message"}`),
	)

	require.Equal(t, ErrorPolicySkipped, result,
		"custom_codes miss + temp_unsched 规则不命中应回到 Skipped")
	require.Equal(t, 0, repo.tempCalls)
}

func TestCheckErrorPolicy_CustomCodesMiss_TempUnschedDisabled(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	// 不开启 temp_unsched
	account := makeCustomCodesAccountForMissTest(false)

	result := svc.CheckErrorPolicy(
		context.Background(),
		account,
		http.StatusBadGateway,
		[]byte(`{"error":"bad gateway"}`),
	)

	require.Equal(t, ErrorPolicySkipped, result,
		"temp_unsched 未启用时不应升级")
	require.Equal(t, 0, repo.tempCalls)
}

// ----- HandleUpstreamError（本次新增的主路对称修复） -----

func TestHandleUpstreamError_CustomCodesMiss_TempUnschedRuleHits(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := makeCustomCodesAccountForMissTest(true)

	shouldDisable := svc.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusBadGateway,
		http.Header{},
		[]byte(`upstream returned bad gateway`),
	)

	require.True(t, shouldDisable,
		"主路：custom_codes miss + temp_unsched 命中应返回 true 触发 failover")
	require.Equal(t, 1, repo.tempCalls)
}

func TestHandleUpstreamError_CustomCodesMiss_TempUnschedRuleMisses(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := makeCustomCodesAccountForMissTest(true)

	shouldDisable := svc.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusBadGateway,
		http.Header{},
		[]byte(`completely different error`),
	)

	require.False(t, shouldDisable,
		"主路：custom_codes miss + 规则不命中应保持 false")
	require.Equal(t, 0, repo.tempCalls)
}

func TestHandleUpstreamError_CustomCodesMiss_TempUnschedDisabled(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := makeCustomCodesAccountForMissTest(false)

	shouldDisable := svc.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusBadGateway,
		http.Header{},
		[]byte(`bad gateway`),
	)

	require.False(t, shouldDisable,
		"主路：temp_unsched 未启用应保持 false")
	require.Equal(t, 0, repo.tempCalls)
}

// 401 守卫：custom_codes 白名单不含 401 时（极少见但合法配置），不走 temp_unsched，
// 让 token 刷新流程接管。
func TestHandleUpstreamError_CustomCodesMiss_401SkipsTempUnsched(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	// 把白名单换成不包含 401
	account := &Account{
		ID:       902,
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"custom_error_codes_enabled": true,
			"custom_error_codes":         []any{float64(429)}, // 只有 429
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules": []any{
				map[string]any{
					"error_code":       float64(401),
					"keywords":         []any{"unauthorized"},
					"duration_minutes": float64(5),
				},
			},
		},
	}

	shouldDisable := svc.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusUnauthorized,
		http.Header{},
		[]byte(`unauthorized`),
	)

	require.False(t, shouldDisable,
		"主路：401 即使规则命中也不应走 temp_unsched，让 token 刷新接管")
	require.Equal(t, 0, repo.tempCalls)
}
