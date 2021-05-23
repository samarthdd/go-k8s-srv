package tracing

import (
	"log"
	"testing"
)

func TestInitlocal(t *testing.T) {

	t.Run("TestInitlocal", func(t *testing.T) {
		testresult, _ := Initlocal("testserv")
		if testresult == nil {

			t.Errorf("testserv(srv) = %d; want nil", testresult)

		} else {
			log.Println("[√] TestInitlocal successfully")

		}
	})
	t.Run("TestInit", func(t *testing.T) {
		testresult, _ := Init("testserv")
		if testresult == nil {

			t.Errorf("testserv(srv) = %d; want nil", testresult)

		} else {
			log.Println("[√] TestInit successfully")

		}
	})
}
