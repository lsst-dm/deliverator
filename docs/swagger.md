


# s3nd|The Deliverator API
  

## Informations

### Version

0.0.0

### License

[GPL-3.0](https://www.gnu.org/licenses/gpl-3.0.en.html)

### Contact

Support  https://github.com/lsst-dm/s3nd

## Content negotiation

### URI Schemes
  * http

### Consumes
  * application/x-www-form-urlencoded

### Produces
  * application/json

## All endpoints

###  uploads

| Method  | URI     | Name   | Summary |
|---------|---------|--------|---------|
| POST | /upload | [post upload](#post-upload) | upload file to S3 |
  


###  version

| Method  | URI     | Name   | Summary |
|---------|---------|--------|---------|
| GET | /version | [get version](#get-version) | report service version and configuration |
  


## Paths

### <span id="get-version"></span> report service version and configuration (*GetVersion*)

```
GET /version
```

#### Produces
  * application/json

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#get-version-200) | OK | OK |  | [schema](#get-version-200-schema) |

#### Responses


##### <span id="get-version-200"></span> 200 - OK
Status: OK

###### <span id="get-version-200-schema"></span> Schema
   
  

[VersionInfo](#version-info)

### <span id="post-upload"></span> upload file to S3 (*PostUpload*)

```
POST /upload
```

#### Consumes
  * application/x-www-form-urlencoded

#### Produces
  * application/json

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| file | `formData` | string | `string` |  | ✓ |  | path to file to upload |
| slug | `formData` | string | `string` |  |  |  | arbitrary string to include in logs |
| uri | `formData` | string | `string` |  | ✓ |  | Destination S3 URI |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#post-upload-200) | OK | OK |  | [schema](#post-upload-200-schema) |
| [400](#post-upload-400) | Bad Request | Bad Request | ✓ | [schema](#post-upload-400-schema) |
| [404](#post-upload-404) | Not Found | Not Found | ✓ | [schema](#post-upload-404-schema) |
| [408](#post-upload-408) | Request Timeout | Request Timeout | ✓ | [schema](#post-upload-408-schema) |
| [500](#post-upload-500) | Internal Server Error | Internal Server Error | ✓ | [schema](#post-upload-500-schema) |
| [504](#post-upload-504) | Gateway Timeout | Gateway Timeout | ✓ | [schema](#post-upload-504-schema) |

#### Responses


##### <span id="post-upload-200"></span> 200 - OK
Status: OK

###### <span id="post-upload-200-schema"></span> Schema
   
  

[RequestStatus200](#request-status200)

##### <span id="post-upload-400"></span> 400 - Bad Request
Status: Bad Request

###### <span id="post-upload-400-schema"></span> Schema
   
  

[RequestStatus400](#request-status400)

###### Response headers

| Name | Type | Go type | Separator | Default | Description |
|------|------|---------|-----------|---------|-------------|
| X-Error | string | `string` |  |  | error message |

##### <span id="post-upload-404"></span> 404 - Not Found
Status: Not Found

###### <span id="post-upload-404-schema"></span> Schema
   
  

[RequestStatus404](#request-status404)

###### Response headers

| Name | Type | Go type | Separator | Default | Description |
|------|------|---------|-----------|---------|-------------|
| X-Error | string | `string` |  |  | error message |

##### <span id="post-upload-408"></span> 408 - Request Timeout
Status: Request Timeout

###### <span id="post-upload-408-schema"></span> Schema
   
  

[RequestStatus408](#request-status408)

###### Response headers

| Name | Type | Go type | Separator | Default | Description |
|------|------|---------|-----------|---------|-------------|
| X-Error | string | `string` |  |  | error message |

##### <span id="post-upload-500"></span> 500 - Internal Server Error
Status: Internal Server Error

###### <span id="post-upload-500-schema"></span> Schema
   
  

[RequestStatus500](#request-status500)

###### Response headers

| Name | Type | Go type | Separator | Default | Description |
|------|------|---------|-----------|---------|-------------|
| X-Error | string | `string` |  |  | error message |

##### <span id="post-upload-504"></span> 504 - Gateway Timeout
Status: Gateway Timeout

###### <span id="post-upload-504-schema"></span> Schema
   
  

[RequestStatus504](#request-status504)

###### Response headers

| Name | Type | Go type | Separator | Default | Description |
|------|------|---------|-----------|---------|-------------|
| X-Error | string | `string` |  |  | error message |

## Models

### <span id="version-info"></span> VersionInfo


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| config | map of string| `map[string]string` |  | |  |  |
| version | string| `string` |  | |  | `0.0.0` |



### <span id="request-status200"></span> requestStatus200


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| code | integer| `int64` |  | |  | `200` |
| msg | string| `string` |  | |  | `upload succeeded` |
| task | [Task](#task)| `Task` |  | |  |  |



### <span id="request-status400"></span> requestStatus400


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| code | integer| `int64` |  | |  | `400` |
| msg | string| `string` |  | |  | `error parsing request: missing field: uri` |
| task | [RequestStatus400Task](#request-status400-task)| `RequestStatus400Task` |  | |  |  |



#### Inlined models

**<span id="request-status400-task"></span> RequestStatus400Task**


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| duration | string| `string` |  | |  | `37.921µs` |
| file | string| `string` |  | |  | `/path/to/file.txt` |
| id | uuid (formatted string)| `strfmt.UUID` |  | |  |  |
| slug | string| `string` |  | | for logging | `Gray Garden Slug` |



### <span id="request-status404"></span> requestStatus404


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| code | integer| `int64` |  | |  | `404` |
| msg | string| `string` |  | |  | `upload failed because the bucket does not exist` |
| task | [Task](#task)| `Task` |  | |  |  |



### <span id="request-status408"></span> requestStatus408


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| code | integer| `int64` |  | |  | `408` |
| msg | string| `string` |  | |  | `upload queue timeout` |
| task | [Task](#task)| `Task` |  | |  |  |



### <span id="request-status500"></span> requestStatus500


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| code | integer| `int64` |  | |  | `500` |
| msg | string| `string` |  | |  | `unknown error` |
| task | [RequestStatus500Task](#request-status500-task)| `RequestStatus500Task` |  | |  |  |



#### Inlined models

**<span id="request-status500-task"></span> RequestStatus500Task**


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| attempts | integer| `int64` |  | |  | `5` |
| duration | string| `string` |  | |  | `37.921µs` |
| duration_seconds | number| `float64` |  | |  | `0.021` |
| file | string| `string` |  | |  | `/path/to/file.txt` |
| id | uuid (formatted string)| `strfmt.UUID` |  | |  |  |
| size_bytes | integer| `int64` |  | |  | `1000` |
| slug | string| `string` |  | | for logging | `Gray Garden Slug` |
| transfer_rate | string| `string` |  | | human friendly | `1000B/s` |
| transfer_rate_mbits | number| `float64` |  | |  | `0.001` |
| upload_parts | integer| `int64` |  | |  | `1` |
| uri | string| `string` |  | |  | `s3://my-bucket/my-key` |



### <span id="request-status504"></span> requestStatus504


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| code | integer| `int64` |  | |  | `504` |
| msg | string| `string` |  | |  | `timeout during upload attempt 2/2` |
| task | [RequestStatus504Task](#request-status504-task)| `RequestStatus504Task` |  | |  |  |



#### Inlined models

**<span id="request-status504-task"></span> RequestStatus504Task**


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| duration | string| `string` |  | |  | `56.115µs` |
| file | string| `string` |  | |  | `/path/to/file.txt` |
| id | uuid (formatted string)| `strfmt.UUID` |  | |  |  |
| slug | string| `string` |  | | for logging | `Gray Garden Slug` |
| uri | string| `string` |  | |  | `s3://my-bucket/my-key` |



### <span id="task"></span> task


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| attempts | integer| `int64` |  | |  | `1` |
| duration | string| `string` |  | | human friendly | `21.916462ms` |
| duration_seconds | number| `float64` |  | |  | `0.021` |
| file | string| `string` |  | |  | `/path/to/file.txt` |
| id | uuid (formatted string)| `strfmt.UUID` |  | |  |  |
| size_bytes | integer| `int64` |  | |  | `1000` |
| slug | string| `string` |  | | for logging | `Gray Garden Slug` |
| transfer_rate | string| `string` |  | | human friendly | `1000B/s` |
| transfer_rate_mbits | number| `float64` |  | |  | `0.001` |
| upload_parts | integer| `int64` |  | |  | `1` |
| uri | string| `string` |  | |  | `s3://my-bucket/my-key` |


