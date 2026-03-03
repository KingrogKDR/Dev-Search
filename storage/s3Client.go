package storage

import (
	"fmt"
	"os"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func GetS3Client() (*minio.Client, error) {
	s3Client, err := minio.New("localhost:9000", &minio.Options{
		Creds:  credentials.NewStaticV4(os.Getenv("USER"), os.Getenv("PASSWORD"), ""),
		Secure: false,
	})
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	return s3Client, nil

}
