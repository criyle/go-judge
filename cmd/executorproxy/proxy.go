// Command executorclient is used to test executor server's grpc call
package main

import (
	"flag"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/criyle/go-judge/pb"
	"github.com/gin-gonic/gin"
	"github.com/golang/protobuf/jsonpb"
	"google.golang.org/grpc"
)

var (
	addr    = flag.String("addr", ":7755", "Rest api server addr")
	srvAddr = flag.String("srvaddr", "localhost:5051", "GRPC server addr")
)

type execProxy struct {
	client pb.ExecutorClient
}

func (p *execProxy) Exec(c *gin.Context) {
	req := new(pb.Request)
	if err := jsonpb.Unmarshal(c.Request.Body, req); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}
	log.Println(req)
	rep, err := p.client.Exec(c, req)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, rep)
}

func (p *execProxy) FileList(c *gin.Context) {
	rep, err := p.client.FileList(c, &pb.Empty{})
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, rep)
}

func (p *execProxy) FileGet(c *gin.Context) {
	type fileURI struct {
		FileID string `uri:"fid"`
	}
	var uri fileURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	fid := &pb.FileID{
		FileID: uri.FileID,
	}
	rep, err := p.client.FileGet(c, fid)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, rep)
}

func (p *execProxy) FilePost(c *gin.Context) {
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

	req := &pb.FileContent{
		Name:    fh.Filename,
		Content: b,
	}
	rep, err := p.client.FileAdd(c, req)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, rep)
}

func (p *execProxy) FileDelete(c *gin.Context) {
	type fileURI struct {
		FileID string `uri:"fid"`
	}
	var uri fileURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	fid := &pb.FileID{
		FileID: uri.FileID,
	}
	rep, err := p.client.FileDelete(c, fid)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, rep)
}

func main() {
	conn, err := grpc.Dial(*srvAddr, grpc.WithInsecure())
	if err != nil {
		log.Fatalln("client", err)
	}
	client := pb.NewExecutorClient(conn)

	p := &execProxy{client: client}

	r := gin.Default()
	r.POST("/exec", p.Exec)
	r.GET("/file", p.FileList)
	r.GET("/file/:fid", p.FileGet)
	r.POST("/file", p.FilePost)
	r.DELETE("/file/:fid", p.FileDelete)

	log.Println(r.Run(*addr))
}
