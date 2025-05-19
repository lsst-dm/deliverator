# s3nd - The Deliverator

![The Deliverator](./images/2b93b4eb-4971-4a1d-b64d-fc5ec046a06c.png)

> The S3 Nexus Deliverator -- "s3nd" to the mortals who tap its command and
> watch bytes ignite—is no ordinary file-pusher; it is a swaggering,
> carbon-fiber katana forged for the cloud-age ronin that moves data at orbital
> velocity and expects reality to keep up. It drop-kicks petabytes across
> Amazon’s S3 strata with the icy calm of a hyper-focused courier, slices
> latency like fresh sashimi, and tattoos every object with cryptographic
> sigils before the competition has even parsed their credentials. Because in
> this fractured mesh of micro-services and franchised LLMs, sovereignty
> belongs to whoever controls the pipeline—and s3nd is the midnight-black
> muscle car that shows up, engine snarling, ready to haul your binary payload
> through rush-hour congestion in under thirty seconds, refund guaranteed,
> pride irrevocable.
>
> -- Hiro Photogonist, Latency Survey of S3 Transfers

## Problem Space

The transfer of data across high latency (> 200ms) long-haul networks where the
data is produced in periodic bursts. Each burst is composed of multiple related
files and the total transfer time for all files within the burst must be
minimized.

## Architecture

The Deliverator design is based on [`lsst-dm/s3daemon`](https://github.com/lsst-dm/s3daemon).

A pool of cached https connection is maintained.
Transfers are aggressively timed out and retried as trade off between efficient use of bandwidth and minimizing the completion time of a file transfer.

## Upload Request Submission

Local files are submitted for upload to a `s3nd` instance via HTTP POST request.
All parameters are `application/x-www-form-urlencoded`, which makes it simple to create a upload request with common tools such as `curl`.

There are two mandatory parameters to make a valid request:

- `file`
    The fully qualified path to the file to be uploaded.
    The file (and its parent directories) must be accessible to the euid/egid the `s3nd` service is running as.
    This parameter is required.
- `uri`
    The URI of the S3 object the file should be uploaded too.
    The destination bucket must already exist.
    Note that there is no checking if the object already exists.
    If the s3 credentials in use by `s3nd` allow an object to be overwritten, `s3nd` will do so.
    This parameter is required.

Example of submitting a file for transfer with `curl`:

```bash
curl -d "file=/tmp/foo" -d "uri=s3://bar/foo" http://localhost:15555/upload
```

The HTTP connection will be held open until by the `s3nd` server until a definitive transfer status (success or error) is returned via HTTP status code.
The Deliverator does not maintain persistent state for queued or in progress uploads.
If the service is restarted, all in progress transfers are abandoned.

> [!IMPORTANT]
> If an HTTP client of `s3nd` disconnects, I.e., the socket is closed, the request is considered abandoned.
> If there is an s3 upload in progress, it will be aborted.

A status message shall always be returned to the client encoded as `application/json`. A `code` with an http status code as integer value and `msg` string will be present.
Other information may be part of the response.

## API Documentation

API documentation is avaiabile from a running `s3nd` instance in `html` format from the endpoint `http://localhost:15555/swagger/`, as well as in [docs/swager.md](docs/swagger.md).

## Example Status Responses

### Successful Upload

```console
 ~ $ curl -vvv -d "file=/tmp/foo" -d "uri=s3://bar/foo" http://localhost:15555/upload
* Host localhost:15555 was resolved.
* IPv6: ::1
* IPv4: 127.0.0.1
*   Trying [::1]:15555...
* connect to ::1 port 15555 from ::1 port 38094 failed: Connection refused
*   Trying 127.0.0.1:15555...
* Connected to localhost (127.0.0.1) port 15555
> POST /upload HTTP/1.1
> Host: localhost:15555
> User-Agent: curl/8.9.1
> Accept: */*
> Content-Length: 30
> Content-Type: application/x-www-form-urlencoded
> 
* upload completely sent off: 30 bytes
< HTTP/1.1 200 OK
< Content-Type: application/json
< Date: Fri, 30 May 2025 17:36:13 GMT
< Content-Length: 198
< 
{"code":200,"msg":"upload succeeded","task":{"id":"0344cbb5-d097-48c4-86ad-75d4eb038cea","uri":"s3://bar/foo","file":"/tmp/foo","duration":"20.021872ms","attempts":1,"transfer_rate":"0.000Mbit/s"}}
* Connection #0 to host localhost left intact
```

## Configuration

```console
Usage of ./s3nd:
  -endpoint-url string
    	S3 Endpoint URL (S3ND_ENDPOINT_URL)
  -host string
    	S3 Daemon Host (S3ND_HOST) (default "localhost")
  -port int
    	S3 Daemon Port (S3ND_PORT) (default 15555)
  -queue-timeout string
    	Queue Timeout waiting for transfer to start (S3ND_QUEUE_TIMEOUT) (default "10s")
  -upload-bwlimit string
    	Upload bandwidth limit in bits per second (S3ND_UPLOAD_BWLIMIT) (default "0")
  -upload-max-parallel int
    	Maximum number of parallel object uploads (S3ND_UPLOAD_MAX_PARALLEL) (default 100)
  -upload-partsize string
    	Upload Part Size (S3ND_UPLOAD_PARTSIZE) (default "5Mi")
  -upload-timeout string
    	Upload Timeout (S3ND_UPLOAD_TIMEOUT) (default "10s")
  -upload-tries int
    	Max number of upload tries (S3ND_UPLOAD_TRIES) (default 1)
  -upload-write-buffer-size string
    	Upload Write Buffer Size (S3ND_UPLOAD_WRITE_BUFFER_SIZE) (default "64Ki")
```

### `AWS_ACCESS_KEY_ID`

Standard `s3` access credentials.

### `AWS_SECRET_ACCESS_KEY`

Standard `s3` access credentials.

### `AWS_REGION`

Some `s3` endpoints require that a region is specified.

### `S3ND_ENDPOINT_URL`

The S3 endpoint to which objects will be transferred.
This must be configured for the service to start.

### `S3ND_HOST`

The address to listen on for transfer submission.

Note that `""` (empty string) or `[::]` are equivalent to `0.0.0.0`.

### `S3ND_PORT`

The TCP port to listen on for transfer submission.

### `S3ND_QUEUE_TIMEOUT`

The amount of time a pending transfer is allowed to be in enqueued after the `S3ND_UPLOAD_MAX_PARALLEL` limit has been reached.
This timeout does not apply once the transfer has started.

Note that enqueued transfers are not persisted across a service restart.

### `S3ND_UPLOAD_BWLIMIT`

The bandwidth rate to request on transfer TCP sockets via `SO_MAX_PACING_RATE`.

E.g.
```
S3ND_UPLOAD_BWLIMIT="100Mi"
S3ND_UPLOAD_MAX_PARALLEL="10"
```

Would limit each TCP stream to 100mbit/s and the aggregate limit would be 1gbit/s.
However, multipart transfers could currently result in a much higher aggregate limit.

### `S3ND_UPLOAD_MAX_PARALLEL`

Maximum number of concurrent object uploads.
An object using multipart transfers is currently considered as 1 object by this quota.

### `S3ND_UPLOAD_PARTSIZE`

A file submission that exceeds this size will be partitioned and uploaded concurrently as an [s3 mulipart upload](https://docs.aws.amazon.com/AmazonS3/latest/userguide/mpuoverview.html)

E.g.
```
S3ND_UPLOAD_PARTSIZE="10Mi"
```
When submitting a 25MiB file for transfer, would result in 3 total parts: 2x 10MiB and 1x 5MiB.

A multipart upload requires additional calls to initiate and finalize the transfer.
This may add undesirable latency.
For example, on a 200ms RTT connection, this would increase the transfer time by *at least* 400ms.

### `S3ND_UPLOAD_TIMEOUT`

The amount of time allowed before aborting a transfer attempt.
If additional "tries" remain, the transfer will be retried.
Retry attempts are not re-enqueued and are always attempted immediately.

E.g.
```
S3ND_UPLOAD_TIMEOUT="3s"
S3ND_UPLOAD_TRIES="10"
```

Could result up in to 30s elapsing before a transfer is reported as failed.

### `S3ND_UPLOAD_TRIES`

The maximum number of upload tries.
A value of 2 means there would be 1 retry attempt after a failure (http error code or timeout).

### `S3ND_UPLOAD_WRITE_BUFFER_SIZE`

Controls the number of bytes written at once to the upload s3 socket(s).
