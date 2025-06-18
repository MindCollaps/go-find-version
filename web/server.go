package web

import (
	"github.com/gin-gonic/gin"
	"go-find-version/utils"
	"net/http"
	"strconv"
)

func Init(port int) {
	r := gin.Default()

	r.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	sPort := strconv.Itoa(port)

	go func() {
		utils.PrintInfo("Server started on port " + sPort)
		if err := r.Run(":" + sPort); err != nil {
			utils.PrintError(err, "Server failed to start")
		}
	}()
}
