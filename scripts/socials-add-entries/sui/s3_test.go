package sui_test

import (
	"log"
	"os"
	"testing"

	"github.com/idivarts/backend-sls/internal/models/trendlybq"
	"github.com/idivarts/backend-sls/scripts/socials-add-entries/sui"
)

func TestUpload(t *testing.T) {
	os.Setenv("S3_BUCKET", "trendly-discovery-bucket")
	os.Setenv("S3_URL", "https://trendly-discovery-bucket.s3.us-east-1.amazonaws.com")

	picture := "https://scontent-lga3-1.cdninstagram.com/v/t51.2885-19/535713954_18333999466204082_726845497106540871_n.jpg?stp=dst-jpg_e0_s150x150_tt6&efg=eyJ2ZW5jb2RlX3RhZyI6InByb2ZpbGVfcGljLmRqYW5nby4xMDgwLmMyIn0&_nc_ht=scontent-lga3-1.cdninstagram.com&_nc_cat=102&_nc_oc=Q6cZ2QFWmNFyoLB8guIWw_qfeEAJYxmzvsriwO1FlTtikhDe-iIZULfBdm2zc4DHcCzucuL_2BJkh41fCu6-rMhfAfdE&_nc_ohc=HoOKA9y5GocQ7kNvwF6Qc3_&_nc_gid=-wjoh7AjBqYW4SL5DytqsA&edm=AOQ1c0wBAAAA&ccb=7-5&oh=00_Afu_TFPqIBfjyTMKMWb8KVpzmYOGaLNazyTPIaIgInh8Tw&oe=69913E25&_nc_sid=8b3546"

	url, err := sui.DownloadAndUploadToS3(picture, "test")
	if err != nil {
		t.Error(err)
	}
	log.Println("Uploaded", url)
}

func TestUpdateQuery(t *testing.T) {
	sql := "DELETE FROM " + trendlybq.SocialsN8NFullTableName + " WHERE last_update_time<1758222410855465"
	log.Println(sql)
}

func TestGetFirestoreAll(t *testing.T) {
	socials, err := trendlybq.SocialsN8N{}.GetPaginatedFromFirestore(0, 700)
	if err != nil {
		t.Error(err)
	}
	log.Println("Total Socials", len(socials))
}

func TestRemoveDataFromFirebase(t *testing.T) {
	socials, err := trendlybq.SocialsN8N{}.GetPaginatedFromFirestore(0, 700)
	if err != nil {
		t.Fatal(err)
	}
	log.Println("Total Socials", len(socials))
	for i, v := range socials {
		err := v.UpdateMinified()
		if err != nil {
			log.Println("Error", i)
		} else {
			log.Println("Updates", i, v)
		}
	}
	log.Println("Updated all")
}
