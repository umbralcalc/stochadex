#!/usr/bin/env python3
"""Generate the affine_sigmoid.onnx model used by the onnx_inference test.

The model computes  y = sigmoid(W @ x + b)  for a 3-vector input and a 2-vector
output, with the weights below. The Go test hard-codes the same W and b and
asserts the ONNX Runtime output matches the hand-computed reference to float
tolerance — the differential test for the frozen-inference partition.

Opset and IR version are pinned low so the committed model loads across a wide
range of ONNX Runtime versions, independent of the newer onnx package that
writes it.

Run:  python3 gen_model.py   (needs `pip install onnx numpy`)
"""

import numpy as np
import onnx
from onnx import TensorProto, helper, numpy_helper

# Weights — kept in sync with affine_sigmoid_test.go.
W = np.array([[1.0, 2.0, 3.0], [4.0, 5.0, 6.0]], dtype=np.float32)  # (2, 3)
B = np.array([0.5, -0.5], dtype=np.float32)                         # (2,)

# Gemm: Y = alpha*(A @ B) + beta*C, with transB=1 so B is stored as (out, in).
gemm = helper.make_node(
    "Gemm",
    inputs=["input", "W", "b"],
    outputs=["affine"],
    alpha=1.0,
    beta=1.0,
    transB=1,
)
sigmoid = helper.make_node("Sigmoid", inputs=["affine"], outputs=["output"])

graph = helper.make_graph(
    nodes=[gemm, sigmoid],
    name="affine_sigmoid",
    inputs=[helper.make_tensor_value_info("input", TensorProto.FLOAT, [1, 3])],
    outputs=[helper.make_tensor_value_info("output", TensorProto.FLOAT, [1, 2])],
    initializer=[
        numpy_helper.from_array(W, name="W"),
        numpy_helper.from_array(B, name="b"),
    ],
)

model = helper.make_model(
    graph,
    opset_imports=[helper.make_opsetid("", 13)],
    ir_version=9,
)
onnx.checker.check_model(model)
onnx.save(model, "affine_sigmoid.onnx")
print("wrote affine_sigmoid.onnx")
