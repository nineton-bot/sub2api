package caiyuntong

import (
	"strconv"
	"testing"
)

// TestBuildBlueLineItems_Invariants 验证财云通的强校验约束：
//
//	SUM(SumAmount) == TotalAmount
//	单行 InvoiceAmount + TaxAmount == 单行 SumAmount
//	header InvoiceAmount + TaxAmount == header TotalAmount
//	|单行 InvoiceAmount × TaxRate - 单行 TaxAmount| ≤ 0.06   ← 财云通 8011 错误
//
// 用容易产生四舍五入尾差的金额（如 ¥99.99 / 3 行）做最严格的覆盖。
func TestBuildBlueLineItems_Invariants(t *testing.T) {
	cases := []struct {
		name     string
		items    []LineItem
		totalIn  float64
		taxRate  float64
	}{
		{"single", []LineItem{{Name: "API充值", PayAmount: 100}}, 100, 0.06},
		{"two_equal", []LineItem{{Name: "a", PayAmount: 50}, {Name: "b", PayAmount: 50}}, 100, 0.06},
		{"three_uneven", []LineItem{{PayAmount: 33.33}, {PayAmount: 33.33}, {PayAmount: 33.33}}, 99.99, 0.06},
		{"three_with_remainder", []LineItem{{PayAmount: 28.25}, {PayAmount: 9.99}, {PayAmount: 1.01}}, 39.25, 0.13},
		{"all_zero_pay_amount_fallback", []LineItem{{Name: "x"}, {Name: "y"}}, 88.50, 0.06},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows, hdrIncl, hdrExcl, hdrTax := buildBlueLineItems(tc.items, tc.totalIn, tc.taxRate, "")

			// 1. header total 等于参数
			if !floatEq(hdrIncl, tc.totalIn) {
				t.Fatalf("hdr incl %.4f != totalIn %.4f", hdrIncl, tc.totalIn)
			}
			// 2. header excl + tax = incl
			if !floatEq(round2(hdrExcl+hdrTax), hdrIncl) {
				t.Fatalf("hdr excl(%.4f)+tax(%.4f)!=incl(%.4f)", hdrExcl, hdrTax, hdrIncl)
			}
			// 3. 行累加 == header
			var sumSum, sumExcl, sumTax float64
			for _, r := range rows {
				s, _ := strconv.ParseFloat(r.SumAmount, 64)
				e, _ := strconv.ParseFloat(r.InvoiceAmount, 64)
				x, _ := strconv.ParseFloat(r.TaxAmount, 64)
				sumSum = round2(sumSum + s)
				sumExcl = round2(sumExcl + e)
				sumTax = round2(sumTax + x)
				// 4. 单行 excl+tax = sum
				if !floatEq(round2(e+x), s) {
					t.Fatalf("row %s: excl(%.4f)+tax(%.4f)!=sum(%.4f)", r.LineNo, e, x, s)
				}
				// 5. 单行税率一致性（财云通 8011 业务校验）
				expectedTax := e * tc.taxRate
				if diff := abs(expectedTax - x); diff > 0.06 {
					t.Fatalf("row %s tax inconsistent: excl=%.4f, rate=%.4f, expected_tax=%.4f, actual_tax=%.4f, diff=%.4f",
						r.LineNo, e, tc.taxRate, expectedTax, x, diff)
				}
			}
			if !floatEq(sumSum, hdrIncl) {
				t.Fatalf("SUM(SumAmount)=%.4f != hdrIncl=%.4f", sumSum, hdrIncl)
			}
			if !floatEq(sumExcl, hdrExcl) {
				t.Fatalf("SUM(InvoiceAmount)=%.4f != hdrExcl=%.4f", sumExcl, hdrExcl)
			}
			if !floatEq(sumTax, hdrTax) {
				t.Fatalf("SUM(TaxAmount)=%.4f != hdrTax=%.4f", sumTax, hdrTax)
			}
		})
	}
}

// TestBuildReturnReceiptItems_NegativeInvariants 退货单（红冲）行明细金额必须全部为负，
// 且累加值等于 header 总额。
func TestBuildReturnReceiptItems_NegativeInvariants(t *testing.T) {
	items := []LineItem{{PayAmount: 33.33}, {PayAmount: 33.33}, {PayAmount: 33.33}}
	totalIncl := -99.99
	rows, hdrIncl, hdrExcl, hdrTax := buildReturnReceiptItems(items, totalIncl, 0.06, "")

	if !floatEq(hdrIncl, totalIncl) {
		t.Fatalf("hdr incl %.4f != totalIn %.4f", hdrIncl, totalIncl)
	}
	if hdrIncl >= 0 || hdrExcl >= 0 || hdrTax >= 0 {
		t.Fatalf("red invoice header amounts must be negative: incl=%.4f excl=%.4f tax=%.4f", hdrIncl, hdrExcl, hdrTax)
	}
	var sumSum float64
	for _, r := range rows {
		s, _ := strconv.ParseFloat(r.SumAmount, 64)
		sumSum = round2(sumSum + s)
		if s >= 0 {
			t.Fatalf("red row %d has non-negative SumAmount %.4f", r.LineNo, s)
		}
	}
	if !floatEq(sumSum, hdrIncl) {
		t.Fatalf("SUM(red SumAmount)=%.4f != hdrIncl=%.4f", sumSum, hdrIncl)
	}
}

func floatEq(a, b float64) bool {
	return abs(a-b) < 0.005
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
