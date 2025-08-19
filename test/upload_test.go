package s3nd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/lsst-dm/s3nd/client"
	"github.com/lsst-dm/s3nd/upload"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	minio "github.com/minio/minio-go/v7"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("POST /upload", func() {
	testFile := "test1"
	f, err := os.CreateTemp("", testFile)
	Expect(err).NotTo(HaveOccurred())
	for i := 0; i < 3; i++ {
		_, err = f.WriteString(testFile + "\n")
		Expect(err).NotTo(HaveOccurred())
	}
	_ = f.Sync()

	It("returns 200", func() {
		resp, err := http.PostForm(s3ndUrl.String()+"/upload",
			url.Values{
				"file": {f.Name()},
				"uri":  {"s3://" + s3ndBucket + "/" + testFile},
				"slug": {"banana"},
			})
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})

	It("created an s3 object", func() {
		o, err := s3.GetObject(context.Background(), s3ndBucket, testFile, minio.GetObjectOptions{})
		Expect(err).NotTo(HaveOccurred())
		defer o.Close()

		data, err := io.ReadAll(o)
		Expect(err).NotTo(HaveOccurred())

		Expect(string(data)).To(MatchRegexp(fmt.Sprintf("(%s\n){3}", testFile)))
	})

	Describe("file parameter", func() {
		It("is required", func() {
			resp, err := http.PostForm(s3ndUrl.String()+"/upload",
				url.Values{
					"uri":  {"s3://" + s3ndBucket + "/" + testFile},
					"slug": {"banana"},
				})
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			status := upload.RequestStatus{}
			body, _ := io.ReadAll(resp.Body)
			_ = json.Unmarshal(body, &status)
			Expect(status.Msg).To(ContainSubstring("bad request: missing field: file"))
		})

		It("must be an absolute path", func() {
			resp, err := http.PostForm(s3ndUrl.String()+"/upload",
				url.Values{
					"file": {"does/not/exist"},
					"uri":  {"s3://" + s3ndBucket + "/" + testFile},
					"slug": {"banana"},
				})
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			status := upload.RequestStatus{}
			body, _ := io.ReadAll(resp.Body)
			_ = json.Unmarshal(body, &status)
			Expect(status.Msg).To(ContainSubstring("bad request: only absolute file paths are supported"))
		})

		It("fails when file can not be stat()'d", func() {
			resp, err := http.PostForm(s3ndUrl.String()+"/upload",
				url.Values{
					"file": {"/does/not/exist"},
					"uri":  {"s3://" + s3ndBucket + "/" + testFile},
					"slug": {"banana"},
				})
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			status := upload.RequestStatus{}
			body, _ := io.ReadAll(resp.Body)
			_ = json.Unmarshal(body, &status)
			Expect(status.Msg).To(ContainSubstring("bad request: could not stat file: \"/does/not/exist\""))
		})
	})

	Describe("uri parameter", func() {
		It("is required", func() {
			resp, err := http.PostForm(s3ndUrl.String()+"/upload",
				url.Values{
					"file": {f.Name()},
					"slug": {"banana"},
				})
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			status := upload.RequestStatus{}
			body, _ := io.ReadAll(resp.Body)
			_ = json.Unmarshal(body, &status)
			Expect(status.Msg).To(ContainSubstring("bad request: missing field: uri"))
		})

		It("fails without valid uri", func() {
			resp, err := http.PostForm(s3ndUrl.String()+"/upload",
				url.Values{
					"file": {f.Name()},
					"uri":  {"\x7f"},
					"slug": {"banana"},
				})
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			status := upload.RequestStatus{}
			body, _ := io.ReadAll(resp.Body)
			_ = json.Unmarshal(body, &status)
			Expect(status.Msg).To(ContainSubstring("bad request: unable to parse URI: \"\\x7f\""))
		})

		It("fails without s3 schema", func() {
			resp, err := http.PostForm(s3ndUrl.String()+"/upload",
				url.Values{
					"file": {f.Name()},
					"uri":  {"http://example.com"},
					"slug": {"banana"},
				})
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			status := upload.RequestStatus{}
			body, _ := io.ReadAll(resp.Body)
			_ = json.Unmarshal(body, &status)
			Expect(status.Msg).To(ContainSubstring("bad request: only s3 scheme is supported: \"http://example.com\""))
		})

		It("fails without host (bucket)", func() {
			resp, err := http.PostForm(s3ndUrl.String()+"/upload",
				url.Values{
					"file": {f.Name()},
					"uri":  {"s3:///" + testFile},
					"slug": {"banana"},
				})
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			status := upload.RequestStatus{}
			body, _ := io.ReadAll(resp.Body)
			_ = json.Unmarshal(body, &status)
			Expect(status.Msg).To(ContainSubstring("bad request: unable to parse bucket from URI: \"s3:///" + testFile + "\""))
		})
	})

	When("bucket does not exist", func() {
		PIt("returns 404", func() {
			resp, err := http.PostForm(s3ndUrl.String()+"/upload",
				url.Values{
					"file": {f.Name()},
					"uri":  {"s3://bucket-does-not-exist/" + testFile},
					"slug": {"banana"},
				})
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
			status := upload.RequestStatus{}
			body, _ := io.ReadAll(resp.Body)
			_ = json.Unmarshal(body, &status)
			Expect(status.Msg).To(ContainSubstring("upload failed because the bucket does not exist"))
		})
	})

	When("client disconnects", func() {
		It("aborts upload", func() {
			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(500*time.Microsecond))
			defer cancel()

			req, err := http.NewRequestWithContext(ctx, "POST", s3ndUrl.String()+"/upload", strings.NewReader(url.Values{
				"file": {f.Name()},
				"uri":  {"s3://" + s3ndBucket + "/" + testFile},
			}.Encode()))
			Expect(err).NotTo(HaveOccurred())

			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			_, err = http.DefaultClient.Do(req)
			// if there was no error, the upload completed before the context deadline
			Expect(err).To(HaveOccurred())

			Eventually(s3ndOutput, time.Second, 50*time.Millisecond).Should(gbytes.Say("upload request aborted because the client disconnected"))
		})
	})
})

var _ = Describe("client.Upload", func() {
	var status *upload.RequestStatus
	var uri url.URL
	var file string
	slug := "banana"
	testFile := "test2"

	It("returns 200", func() {
		f, err := os.CreateTemp("", testFile)
		Expect(err).NotTo(HaveOccurred())
		defer os.Remove(f.Name())
		defer f.Close()
		for i := 0; i < 3; i++ {
			_, err = f.WriteString(testFile + "\n")
			Expect(err).NotTo(HaveOccurred())
		}
		_ = f.Sync()

		s3nd := client.NewClient(s3ndUrl)
		file = f.Name()
		uri = url.URL{
			Scheme: "s3",
			Host:   s3ndBucket,
			Path:   "/" + testFile,
		}

		status, err = s3nd.Upload(context.Background(), file, uri, slug)
		Expect(err).NotTo(HaveOccurred())
		Expect(status.Code).To(Equal(http.StatusOK))
	})

	It("created an s3 object", func() {
		o, err := s3.GetObject(context.Background(), s3ndBucket, testFile, minio.GetObjectOptions{})
		Expect(err).NotTo(HaveOccurred())
		defer o.Close()

		data, err := io.ReadAll(o)
		Expect(err).NotTo(HaveOccurred())

		Expect(string(data)).To(MatchRegexp(fmt.Sprintf("(%s\n){3}", testFile)))
	})

	It("has a populated RequestStatus", func() {
		Expect(status.Code).To(Equal(http.StatusOK))
		Expect(status.Msg).To(Equal("upload succeeded"))
		Expect(status.Task).ToNot(BeNil())

		task := status.Task
		Expect(task.Id).ToNot(BeEmpty())
		Expect(task.Uri).To(Equal(&upload.RequestURL{URL: uri}))
		Expect(task.Bucket).To(BeNil()) // unset
		Expect(task.Key).To(BeNil())    // unset
		Expect(task.File).To(Equal(&file))
		Expect(task.StartTime.IsZero()).To(BeTrue()) // unset
		Expect(task.EndTime.IsZero()).To(BeTrue())   // unset
		Expect(task.Duration).ToNot(BeEmpty())
		Expect(task.DurationSeconds).ToNot(BeZero())
		Expect(task.Attempts).To(Equal(1))
		Expect(task.SizeBytes).ToNot(BeZero())
		Expect(task.UploadParts).To(Equal(int64(1)))
		Expect(task.TransferRate).ToNot(BeEmpty())
		Expect(task.TransferRateMbits).ToNot(BeZero())
		Expect(task.Slug).To(Equal(&slug))
	})
})

var _ = Describe("client.UploadMulti", func() {
	var statuses *[]*client.UploadStatus
	testFiles := []string{
		"upload_multi1",
		"upload_multi2",
		"upload_multi3",
	}
	slug := "garden"

	It("returns 200", func() {
		uploads := map[string]url.URL{}

		for _, testFile := range testFiles {
			f, err := os.CreateTemp("", testFile)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(f.Name())
			defer f.Close()
			for i := 0; i < 3; i++ {
				_, err = f.WriteString(testFile + "\n")
				Expect(err).NotTo(HaveOccurred())
			}
			_ = f.Sync()
			uploads[f.Name()] = url.URL{
				Scheme: "s3",
				Host:   s3ndBucket,
				Path:   "/" + testFile,
			}
		}

		s3nd := client.NewClient(s3ndUrl)

		var err error
		statuses, err = s3nd.UploadMulti(context.TODO(), uploads, slug)
		Expect(err).NotTo(HaveOccurred())
	})

	It("created s3 object(s))", func() {
		for _, testFile := range testFiles {
			o, err := s3.GetObject(context.Background(), s3ndBucket, testFile, minio.GetObjectOptions{})
			Expect(err).NotTo(HaveOccurred())
			defer o.Close()

			data, err := io.ReadAll(o)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(MatchRegexp(fmt.Sprintf("(%s\n){3}", testFile)))
		}
	})

	It("has populated UploadStatus(es)", func() {
		Expect(len(*statuses)).To(Equal(len(testFiles)))

		for _, uStatus := range *statuses {
			Expect(uStatus).ToNot(BeNil())

			Expect(uStatus.RequestStatus).ToNot(BeNil())
			Expect(uStatus.Error).To(BeNil())
			Expect(uStatus.RequestStatus.Code).To(Equal(http.StatusOK))
			Expect(uStatus.RequestStatus.Task).ToNot(BeNil())

			task := uStatus.RequestStatus.Task
			Expect(task.Id).ToNot(BeEmpty())
			Expect(task.Uri).ToNot(BeNil())
			Expect(task.Bucket).To(BeNil()) // unset
			Expect(task.Key).To(BeNil())    // unset
			Expect(task.File).ToNot(BeNil())
			Expect(task.StartTime).To(Equal(time.Time{})) // unset
			Expect(task.EndTime).To(Equal(time.Time{}))   // unset
			Expect(task.Duration).ToNot(BeEmpty())
			Expect(task.DurationSeconds).ToNot(BeZero())
			Expect(task.Attempts).To(Equal(1))
			Expect(task.SizeBytes).ToNot(BeZero())
			Expect(task.UploadParts).To(Equal(int64(1)))
			Expect(task.TransferRate).ToNot(BeEmpty())
			Expect(task.TransferRateMbits).ToNot(BeZero())
			Expect(task.Slug).To(Equal(&slug))
		}
	})
})
