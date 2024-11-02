package attachments

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gin-gonic/gin"
)

func S3UploadHandler(ctx *gin.Context) {
	filename := ctx.Query("filename")
	if filename == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"message": "Filename is required"})
		return
	}

	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	svc := s3.New(sess)

	// Get s3 bucket name from environment variable
	bucketName := os.Getenv("ATTACHMENT_S3_BUCKET_NAME")
	domainUrl := os.Getenv("ATTACHMENT_CF_DISTRIBUTION_URL")

	filename = fmt.Sprintf("file_%d_%s", time.Now().Unix(), filename)
	imageUrl := fmt.Sprintf("%s/uploads/%s", domainUrl, filename)

	req, _ := svc.PutObjectRequest(&s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(fmt.Sprintf("uploads/%s", filename)),
	})
	url, err := req.Presign(15 * time.Minute)
	if err != nil {
		log.Println("Failed to sign request", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"uploadUrl": url, "attachmentUrl": imageUrl})
}
