package main

import (
	"fmt"
	"io/ioutil"
	"mime"
	"net/http"
	"path"

	"github.com/gin-gonic/gin"
)

func fileGet(c *gin.Context) {
	ids := fs.List()
	c.JSON(http.StatusOK, ids)
}

func filePost(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	f, err := fh.Open()
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	b, err := ioutil.ReadAll(f)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	id, err := fs.Add(fh.Filename, b)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, id)
}

func fileIDGet(c *gin.Context) {
	type fileURI struct {
		FileID string `uri:"fid"`
	}
	var uri fileURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	f := fs.Get(uri.FileID)
	if f == nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	content, err := f.Content()
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	typ := mime.TypeByExtension(path.Ext(f.Name()))
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", f.Name()))
	c.Data(http.StatusOK, typ, content)
}

func fileIDDelete(c *gin.Context) {
	type fileURI struct {
		FileID string `uri:"fid"`
	}
	var uri fileURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	ok := fs.Remove(uri.FileID)
	if !ok {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.Status(http.StatusOK)
}
