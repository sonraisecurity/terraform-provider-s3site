package s3site

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/mholt/archiver"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

var tempDir string = "/tmp/s3site"

func cleanTmp() error {
	return os.RemoveAll(tempDir)
}

func prepareTmp() error {
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		return os.Mkdir(tempDir, 0755)
	}

	return nil
}

type S3Helper struct {
	session  *session.Session
	uploader *s3manager.Uploader
}

func NewS3Helper(session *session.Session) *S3Helper {
	s3Helper := S3Helper{
		session: session,
	}

	uploader := s3manager.NewUploader(session)

	s3Helper.uploader = uploader

	return &s3Helper
}

func (s3Helper S3Helper) ListS3Objects(bucket string) (*s3.ListObjectsV2Output, error) {
	s3conn := s3.New(s3Helper.session)

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	}

	return s3conn.ListObjectsV2(input)
}

func (s3Helper S3Helper) DeleteAllObjects(bucket string) error {
	listObjectResponse, err := s3Helper.ListS3Objects(bucket)

	if err != nil {
		return err
	}

	var keys []string
	for _, object := range listObjectResponse.Contents {
		keys = append(keys, *object.Key)
	}

	return s3Helper.DeleteObjects(bucket, keys)
}

func (s3Helper S3Helper) BulkUploadS3Objects(fileMap map[string]fileInfo, bucket string) error {

	for _, fileInfo := range fileMap {
		fileData, err := ioutil.ReadFile(fileInfo.FullPath)
		if err != nil {
			return err
		}

		contentType := http.DetectContentType(fileData)
		reader := bytes.NewReader(fileData)

		uploadInput := &s3manager.UploadInput{
			Bucket:      &bucket,
			Key:         &fileInfo.RelativePath,
			Body:        reader,
			ContentType: &contentType,
		}

		_, uploaderErr := s3Helper.uploader.Upload(uploadInput)
		if uploaderErr != nil {
			return uploaderErr
		}
	}

	return nil
}

func (s3Helper S3Helper) PutFile(fi fileInfo, bucket string) error {
	fileData, err := ioutil.ReadFile(fi.FullPath)
	if err != nil {
		return err
	}

	contentType := http.DetectContentType(fileData)
	reader := bytes.NewReader(fileData)

	uploadInput := &s3manager.UploadInput{
		Bucket:      &bucket,
		Key:         &fi.RelativePath,
		Body:        reader,
		ContentType: &contentType,
	}

	_, uploaderErr := s3Helper.uploader.Upload(uploadInput)
	if uploaderErr != nil {
		return uploaderErr
	}

	return nil
}

func (s3Helper S3Helper) DeleteObjects(bucket string, keys []string) error {
	svc := s3.New(s3Helper.session)

	for _, key := range keys {
		log.Printf("[DEBUG] Deleting key. bucket=%s, key=%s", bucket, key)
		_, err := svc.DeleteObject(&s3.DeleteObjectInput{
			Bucket: &bucket,
			Key:    &key,
		})

		if err != nil {
			return err
		}
	}

	return nil
}

func cleanS3ETag(eTag string) string {
	return strings.Trim(eTag, "\\\"")
}

func extractArchive(archive string) ([]fileInfo, error) {
	log.Printf("[DEBUG] Extracting archive. path=%s", archive)

	extractedDir := fmt.Sprintf("%s/site/", tempDir)
	if err := archiver.Zip.Open(archive, extractedDir); err != nil {
		return nil, err
	}

	var fileList []fileInfo
	if err := filepath.Walk(extractedDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		fileList = append(fileList, fileInfo{
			FullPath:     path,
			RelativePath: strings.TrimPrefix(path, extractedDir),
			FileInfo:     info,
		})

		return nil
	}); err != nil {
		return nil, err
	}

	return fileList, nil
}

type fileInfo struct {
	FullPath     string
	RelativePath string
	FileInfo     os.FileInfo
	Hash         string
}

func (f fileInfo) getMd5Checksum() (string, error) {
	content, err := ioutil.ReadFile(f.FullPath)
	if err != nil {
		return "", err
	}

	size := int64(len(content))
	contentToHash := content
	parts := 0

	if int64(size) > partSize {
		var pos int64 = 0
		contentToHash = make([]byte, 0)
		for size > pos {
			endpos := pos + partSize
			if endpos >= size {
				endpos = size
			}
			hash := md5.Sum(content[pos:endpos])
			contentToHash = append(contentToHash, hash[:]...)
			pos += partSize
			parts += 1
		}
	}

	hash := md5.Sum(contentToHash)
	etag := fmt.Sprintf("%x", hash)
	if parts > 0 {
		etag += fmt.Sprintf("-%d", parts)
	}
	return etag, nil
}

func encodeKey(key string) string {
	return strings.Replace(key, ".", "%%", -1)
}

func decodeKey(key string) string {
	return strings.Replace(key, "%%", ".", -1)
}
