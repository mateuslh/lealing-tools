package main

import (
	"testing"

	"github.com/mateuslh/lealing-tools/internal/protocoltest"
)

func TestConformidadeScreenV1(t *testing.T) {
	original := modelFactory
	modelFactory = protocoltest.Factory
	defer func() { modelFactory = original }()
	if err := protocoltest.Check("http-probe", runProtocol); err != nil {
		t.Fatal(err)
	}
}
