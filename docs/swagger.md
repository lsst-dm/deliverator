


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
| POST | / | [post](#post) | upload file to S3 |
  


## Paths

### <span id="post"></span> upload file to S3 (*Post*)

```
POST /
```

#### Consumes
  * application/x-www-form-urlencoded

#### Produces
  * application/json

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| file | `formData` | string | `string` |  | ✓ |  | path to file to upload |
| uri | `formData` | string | `string` |  | ✓ |  | Destination S3 URI |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#post-200) | OK | OK |  | [schema](#post-200-schema) |
| [400](#post-400) | Bad Request | Bad Request | ✓ | [schema](#post-400-schema) |
| [500](#post-500) | Internal Server Error | Internal Server Error | ✓ | [schema](#post-500-schema) |
| [504](#post-504) | Gateway Timeout | Gateway Timeout | ✓ | [schema](#post-504-schema) |

#### Responses


##### <span id="post-200"></span> 200 - OK
Status: OK

###### <span id="post-200-schema"></span> Schema
   
  

[RequestStatus200](#request-status200)

##### <span id="post-400"></span> 400 - Bad Request
Status: Bad Request

###### <span id="post-400-schema"></span> Schema
   
  

[RequestStatus400](#request-status400)

###### Response headers

| Name | Type | Go type | Separator | Default | Description |
|------|------|---------|-----------|---------|-------------|
| X-Error | string | `string` |  |  | error message |

##### <span id="post-500"></span> 500 - Internal Server Error
Status: Internal Server Error

###### <span id="post-500-schema"></span> Schema
   
  

[RequestStatus500](#request-status500)

###### Response headers

| Name | Type | Go type | Separator | Default | Description |
|------|------|---------|-----------|---------|-------------|
| X-Error | string | `string` |  |  | error message |

##### <span id="post-504"></span> 504 - Gateway Timeout
Status: Gateway Timeout

###### <span id="post-504-schema"></span> Schema
   
  

[RequestStatus504](#request-status504)

###### Response headers

| Name | Type | Go type | Separator | Default | Description |
|------|------|---------|-----------|---------|-------------|
| X-Error | string | `string` |  |  | error message |

## Models

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



### <span id="request-status500"></span> requestStatus500


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| code | integer| `int64` |  | |  | `500` |
| msg | string| `string` |  | |  | `upload attempt 5/5 timeout: operation error S3: PutObject, context deadline exceeded` |
| task | [RequestStatus500Task](#request-status500-task)| `RequestStatus500Task` |  | |  |  |



#### Inlined models

**<span id="request-status500-task"></span> RequestStatus500Task**


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| attempts | integer| `int64` |  | |  | `5` |
| duration | string| `string` |  | |  | `37.921µs` |
| file | string| `string` |  | |  | `/path/to/file.txt` |
| id | uuid (formatted string)| `strfmt.UUID` |  | |  |  |
| uri | string| `string` |  | |  | `s3://my-bucket/my-key` |



### <span id="request-status504"></span> requestStatus504


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| code | integer| `int64` |  | |  | `504` |
| msg | string| `string` |  | |  | `upload queue timeout: context deadline exceeded` |
| task | [RequestStatus504Task](#request-status504-task)| `RequestStatus504Task` |  | |  |  |



#### Inlined models

**<span id="request-status504-task"></span> RequestStatus504Task**


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| duration | string| `string` |  | |  | `56.115µs` |
| file | string| `string` |  | |  | `/path/to/file.txt` |
| id | uuid (formatted string)| `strfmt.UUID` |  | |  |  |
| uri | string| `string` |  | |  | `s3://my-bucket/my-key` |



### <span id="task"></span> task


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| attempts | integer| `int64` |  | |  | `1` |
| duration | string| `string` |  | |  | `21.916462ms` |
| file | string| `string` |  | |  | `/path/to/file.txt` |
| id | uuid (formatted string)| `strfmt.UUID` |  | |  |  |
| uri | string| `string` |  | |  | `s3://my-bucket/my-key` |


