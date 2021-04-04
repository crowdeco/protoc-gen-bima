## protoc-gen-bima

Plugin ini digunakan bersamaan dengan skeleton bima

- Install

```
go get github.com/crowdeco/protoc-gen-bima
```

- Modifikasi file proto

```
syntax = "proto3";

package grpcs;

import ...
import "protoc-gen-bima/options/gorm.proto";

option go_package = ".;grpcs";

message Category {
    option (gorm.opts) = {
        model: "github.com/crowdeco/skeleton/categories/models;Category"
    };
    string id = 1;
    string name = 2;
}
```

- Tambahkan ke proto_gen.sh

```
protoc -Iprotos -Ilibs --bima_out=protos/builds protos/*.proto
```

- Hasil generated file proto.pb.bima.go

```
package grpcs

import (
	_ "github.com/crowdeco/protoc-gen-bima/options"
	models "github.com/crowdeco/skeleton/categories/models"
	_ "github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2/options"
	_ "google.golang.org/genproto/googleapis/api/annotations"
	http "net/http"
)

type CategoryModel = models.Category

func (x *Category) Bind(v *models.Category) {
	to, from := v, x
	to.Id = from.Id
	to.Name = from.Name
}

func (x *Category) ToModel() models.Category {
	v := models.Category{}
	x.Bind(&v)
	return v
}

func (x *Category) Bundle(v *models.Category) {
	to, from := x, v
	to.Id = from.Id
	to.Name = from.Name
}

func (x *Category) CategoryResponseStatusOK() (*CategoryResponse, error) {
	return &CategoryResponse{
		Code: http.StatusOK,
		Data: x,
	}, nil
}

func (x *Category) CategoryResponseStatusCreated() (*CategoryResponse, error) {
	return &CategoryResponse{
		Code: http.StatusCreated,
		Data: x,
	}, nil
}

func (x *Category) CategoryResponseStatusNoContent() (*CategoryResponse, error) {
	return &CategoryResponse{
		Code: http.StatusNoContent,
	}, nil
}

func (x *Category) CategoryResponseStatusBadRequest(err error) (*CategoryResponse, error) {
	return &CategoryResponse{
		Code:    http.StatusBadRequest,
		Data:    x,
		Message: err.Error(),
	}, nil
}

func (x *Category) CategoryResponseStatusNotFound(err error) (*CategoryResponse, error) {
	return &CategoryResponse{
		Code:    http.StatusNotFound,
		Data:    x,
		Message: err.Error(),
	}, nil
}

func CategoryResponseStatusOK(d *Category) (*CategoryResponse, error) {
	return &CategoryResponse{
		Code: http.StatusOK,
		Data: d,
	}, nil
}

func CategoryResponseStatusCreated(d *Category) (*CategoryResponse, error) {
	return &CategoryResponse{
		Code: http.StatusCreated,
		Data: d,
	}, nil
}

func CategoryResponseStatusNoContent(d *Category) (*CategoryResponse, error) {
	return &CategoryResponse{
		Code: http.StatusNoContent,
	}, nil
}

func CategoryResponseStatusBadRequest(d *Category, err error) (*CategoryResponse, error) {
	return &CategoryResponse{
		Code:    http.StatusBadRequest,
		Data:    d,
		Message: err.Error(),
	}, nil
}

func CategoryResponseStatusNotFound(d *Category, err error) (*CategoryResponse, error) {
	return &CategoryResponse{
		Code:    http.StatusNotFound,
		Data:    d,
		Message: err.Error(),
	}, nil
}

func CategoryPaginatedResponseStatusOK(d []*Category) (*CategoryPaginatedResponse, error) {
	return &CategoryPaginatedResponse{
		Code: http.StatusOK,
		Data: d,
	}, nil
}

func CategoryPaginatedResponseStatusCreated(d []*Category) (*CategoryPaginatedResponse, error) {
	return &CategoryPaginatedResponse{
		Code: http.StatusCreated,
		Data: d,
	}, nil
}

func CategoryPaginatedResponseStatusNoContent(d []*Category) (*CategoryPaginatedResponse, error) {
	return &CategoryPaginatedResponse{
		Code: http.StatusNoContent,
	}, nil
}

func CategoryPaginatedResponseStatusBadRequest(d []*Category, err error) (*CategoryPaginatedResponse, error) {
	return &CategoryPaginatedResponse{
		Code: http.StatusBadRequest,
		Data: d,
	}, nil
}

func CategoryPaginatedResponseStatusNotFound(d []*Category, err error) (*CategoryPaginatedResponse, error) {
	return &CategoryPaginatedResponse{
		Code: http.StatusNotFound,
		Data: d,
	}, nil
}
```