package main

import (
	"fmt"
	"io/ioutil"
	"mime"
	"net/http"
	"path"

	"github.com/criyle/go-judge/filestore"
	"github.com/gin-gonic/gin"
)

type fileHandle struct {
	fs filestore.FileStore
}

func (f *fileHandle) fileGet(c *gin.Context) {
	ids := f.fs.List()
	c.JSON(http.StatusOK, ids)
}

func (f *fileHandle) filePost(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	fi, err := fh.Open()
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	b, err := ioutil.ReadAll(fi)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	id, err := f.fs.Add(fh.Filename, b)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, id)
}

func (f *fileHandle) fileIDGet(c *gin.Context) {
	type fileURI struct {
		FileID string `uri:"fid"`
	}
	var uri fileURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	file := f.fs.Get(uri.FileID)
	if file == nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	content, err := file.Content()
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	typ := mime.TypeByExtension(path.Ext(file.Name()))
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", file.Name()))
	c.Data(http.StatusOK, typ, content)
}

func (f *fileHandle) fileIDDelete(c *gin.Context) {
	type fileURI struct {
		FileID string `uri:"fid"`
	}
	var uri fileURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	ok := f.fs.Remove(uri.FileID)
	if !ok {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.Status(http.StatusOK)
}
