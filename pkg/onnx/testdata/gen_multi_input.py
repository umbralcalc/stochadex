#!/usr/bin/env python3
"""Generate affine_theta_sigmoid.onnx: a two-input model for the multi-input test.

Computes  y = sigmoid(W @ features + theta)  where `features` (a 3-vector) is the
data input and `theta` (a 2-vector) is a *parameter* input — the case that lets the
framework's optimisation / SBI tools tune a model's parameters by feeding them as
just another params vector. The Go test hard-codes the same W and asserts the ONNX
Runtime output matches sigmoid(W @ features + theta) to float tolerance.

Opset and IR version are pinned low so the committed model loads across a wide
range of ONNX Runtime versions.

Run:  python3 gen_multi_input.py   (needs `pip install onnx numpy`)
"""

import numpy as np
import onnx
from onnx import TensorProto, helper, numpy_helper

# Weights — kept in sync with multi_input_test.go.
W = np.array([[1.0, 2.0, 3.0], [4.0, 5.0, 6.0]], dtype=np.float32)  # (2, 3)

matmul = helper.make_node(
    "Gemm", inputs=["features", "W"], outputs=["affine"],
    alpha=1.0, beta=0.0, transB=1,
)
add = helper.make_node("Add", inputs=["affine", "theta"], outputs=["biased"])
sigmoid = helper.make_node("Sigmoid", inputs=["biased"], outputs=["output"])

graph = helper.make_graph(
    nodes=[matmul, add, sigmoid],
    name="affine_theta_sigmoid",
    inputs=[
        helper.make_tensor_value_info("features", TensorProto.FLOAT, [1, 3]),
        helper.make_tensor_value_info("theta", TensorProto.FLOAT, [1, 2]),
    ],
    outputs=[helper.make_tensor_value_info("output", TensorProto.FLOAT, [1, 2])],
    initializer=[numpy_helper.from_array(W, name="W")],
)

model = helper.make_model(
    graph, opset_imports=[helper.make_opsetid("", 13)], ir_version=9
)
onnx.checker.check_model(model)
onnx.save(model, "affine_theta_sigmoid.onnx")
print("wrote affine_theta_sigmoid.onnx")
