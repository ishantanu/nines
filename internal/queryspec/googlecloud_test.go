package queryspec

import "testing"

func TestGoogleCloudCountFilterSubset(t *testing.T) {
	valid := `metric.type = "custom.googleapis.com/nines/requests" AND resource.type = "global" AND metric.labels.result = "good"`
	if fields, err := GoogleCloudFilter(valid, 60); err != nil || fields["metric.labels.result"] != "good" {
		t.Fatal(fields, err)
	}
	for _, q := range []string{
		`metric.type = "custom.googleapis.com/nines/requests"`,
		valid + ` OR project = "other"`, valid + ` AND project = "other"`, valid + ` AND metric.labels.result = "bad"`,
		`metric.type = starts_with("custom") AND resource.type = "global"`,
		valid + "\n", valid + ` AND metric.labels.secret != "x"`,
	} {
		if _, err := GoogleCloudFilter(q, 60); err == nil {
			t.Fatal("unsupported filter accepted", q)
		}
	}
	if _, err := GoogleCloudFilter(valid, 15); err == nil {
		t.Fatal("sub-minute unsupported step")
	}
	quoted := `metric.type = "custom.googleapis.com/nines/requests" AND resource.type = "global" AND metric.labels.message = "A AND B"`
	if _, err := GoogleCloudFilter(quoted, 60); err != nil {
		t.Fatal(err)
	}
}
