package restexecutor

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"

	"github.com/criyle/go-judge/envexec"
	"github.com/criyle/go-judge/filestore"
	"github.com/gin-gonic/gin"
)

type fileHandle struct {
	fs filestore.FileStore
}

// NewFileHandle creates a new file handle
func NewFileHandle(fs filestore.FileStore) Register {
	return &fileHandle{
		fs: fs,
	}
}

func (f *fileHandle) Register(r *gin.Engine) {
	// File handle
	r.GET("/file", f.fileGet)
	r.POST("/file", f.filePost)
	r.GET("/file/:fid", f.fileIDGet)
	r.DELETE("/file/:fid", f.fileIDDelete)
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
	sf, err := f.fs.New()
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	defer sf.Close()

	if _, err := sf.ReadFrom(fi); err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	id, err := f.fs.Add(fh.Filename, sf.Name())
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

	name, file := f.fs.Get(uri.FileID)
	if file == nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	typ := mime.TypeByExtension(path.Ext(name))
	c.Header("Content-Type", typ)

	fi, ok := file.(*envexec.FileInput) // fast path
	if ok {
		c.FileAttachment(fi.Path, name)
		return
	}

	r, err := envexec.FileToReader(file)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	defer r.Close()

	content, err := io.ReadAll(r)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", name))
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
