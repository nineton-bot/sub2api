package caiyuntong

import "testing"

// TestJoinEndpoint 覆盖管理员可能填写的各种 Base URL 形态，
// 确保最终拼出来的 URL 不会重复 "/invoice/createInvoice" 这种路径段。
func TestJoinEndpoint(t *testing.T) {
	cases := []struct {
		name string
		base string
		path string
		want string
	}{
		{
			"root_endpoint",
			"https://api-dataservice.bigfintax.com/",
			"/invoice/createInvoice",
			"https://api-dataservice.bigfintax.com/invoice/createInvoice",
		},
		{
			"root_no_trailing_slash",
			"https://api-dataservice.bigfintax.com",
			"/invoice/createInvoice",
			"https://api-dataservice.bigfintax.com/invoice/createInvoice",
		},
		{
			"with_inv_prefix",
			"https://d-k8s-xxp-ports-fp.bigfintax.com/inv-xx-ports",
			"/invoice/createInvoice",
			"https://d-k8s-xxp-ports-fp.bigfintax.com/inv-xx-ports/invoice/createInvoice",
		},
		{
			"with_inv_prefix_trailing_slash",
			"https://d-k8s-xxp-ports-fp.bigfintax.com/inv-xx-ports/",
			"/invoice/createInvoice",
			"https://d-k8s-xxp-ports-fp.bigfintax.com/inv-xx-ports/invoice/createInvoice",
		},
		// 管理员误填具体 endpoint：必须自动剥除
		{
			"admin_pasted_full_createInvoice_path",
			"https://d-k8s-xxp-ports-fp.bigfintax.com/inv-xx-ports/invoice/createInvoice",
			"/invoice/createInvoice",
			"https://d-k8s-xxp-ports-fp.bigfintax.com/inv-xx-ports/invoice/createInvoice",
		},
		{
			"admin_pasted_full_with_slash",
			"https://api-dataservice.bigfintax.com/invoice/createInvoice/",
			"/invoice/createInvoice",
			"https://api-dataservice.bigfintax.com/invoice/createInvoice",
		},
		// query 串误填
		{
			"endpoint_with_garbage_query",
			"https://api-dataservice.bigfintax.com/?foo=bar",
			"/invoice/createInvoice",
			"https://api-dataservice.bigfintax.com/invoice/createInvoice",
		},
		// 不同 path 不会被误剥
		{
			"different_path_no_dedup",
			"https://api-dataservice.bigfintax.com/inv-xx-ports/invoice/createInvoice",
			"/redInfoBill/apply",
			"https://api-dataservice.bigfintax.com/inv-xx-ports/invoice/createInvoice/redInfoBill/apply",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := joinEndpoint(tc.base, tc.path)
			if got != tc.want {
				t.Fatalf("joinEndpoint(%q, %q)\n  got:  %s\n  want: %s",
					tc.base, tc.path, got, tc.want)
			}
		})
	}
}
