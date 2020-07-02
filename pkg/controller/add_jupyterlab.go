package controller

import (
	"github.com/atef23/jupyter-lab-operator/pkg/controller/jupyterlab"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, jupyterlab.Add)
}
