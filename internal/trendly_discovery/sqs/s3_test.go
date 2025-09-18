package trendly_discovery_sqs_test

import (
	"os"
	"testing"

	trendly_discovery_sqs "github.com/idivarts/backend-sls/internal/trendly_discovery/sqs"
)

func TestUpload(t *testing.T) {
	// S3_BUCKET: trendly-discovery-bucket
	// S3_URL: https://trendly-discovery-bucket.s3.us-east-1.amazonaws.com

	os.Setenv("S3_BUCKET", "trendly-discovery-bucket")
	os.Setenv("S3_URL", "https://trendly-discovery-bucket.s3.us-east-1.amazonaws.com")

	// picture := "https://instagram.fccu27-1.fna.fbcdn.net/v/t51.2885-19/311454657_170545628899938_4001728139220018973_n.jpg?efg=eyJ2ZW5jb2RlX3RhZyI6InByb2ZpbGVfcGljLmRqYW5nby4xMDgwLmMyIn0&_nc_ht=instagram.fccu27-1.fna.fbcdn.net&_nc_cat=106&_nc_oc=Q6cZ2QG36z__HDB8I7sZbYHsX4bJ5cBNQ5bvw2GzPZZ16Sb9fwuHNdFBoGOnUMJ2ov9ysVdPcveGr5Z1Q5b-QeIKpQh8&_nc_ohc=6nJ_ib3-0CwQ7kNvwFXKBez&_nc_gid=gXfAAQp5TmSYLR_7yuUatg&edm=AP4sbd4BAAAA&ccb=7-5&oh=00_AfZlOM3sXuzsrwIHnjW-ij-GF_tb4toln35GFQEmtY74Dg&oe=68CDA291&_nc_sid=7a9f4b"

	trendly_discovery_sqs.MoveImagesToS3("46067716-b467-5199-a0e0-d6c4e1b37143")
}
