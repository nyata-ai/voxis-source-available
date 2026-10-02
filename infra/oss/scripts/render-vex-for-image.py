#!/usr/bin/env python3
"""Bind reviewed OpenVEX statements to exact packages in one scanned image."""

import json
import subprocess
import sys
import tempfile
from pathlib import Path

if len(sys.argv) != 6 or sys.argv[5] != "--package-products":
    raise SystemExit(
        "usage: render-vex-for-image.py TRIVY IMAGE INPUT_VEX OUTPUT_VEX --package-products"
    )

trivy, image, input_vex, output_vex = sys.argv[1:5]
with tempfile.TemporaryDirectory() as directory:
    bom = Path(directory) / "image.cdx.json"
    subprocess.run(
        [trivy, "image", "--scanners", "vuln", "--format", "cyclonedx", "--output", str(bom), image],
        check=True,
    )
    image_bom = json.loads(bom.read_text(encoding="utf-8"))

component = image_bom.get("metadata", {}).get("component")
if not isinstance(component, dict) or component.get("name") != image:
    raise SystemExit("Trivy CycloneDX output does not identify the expected local image")

image_packages = {
    package.get("purl")
    for package in image_bom.get("components", [])
    if isinstance(package, dict) and isinstance(package.get("purl"), str)
}
document = json.loads(Path(input_vex).read_text(encoding="utf-8"))
statements = document.get("statements")
if not isinstance(statements, list) or not statements:
    raise SystemExit("the OpenVEX document has no statements")

for statement in statements:
    products = statement.get("products")
    if not isinstance(products, list) or len(products) != 1:
        raise SystemExit("each reviewed OpenVEX statement must have exactly one product")
    product = products[0]
    if not isinstance(product, dict) or set(product) != {"@id", "subcomponents"}:
        raise SystemExit("each reviewed OpenVEX product must have only an image and its packages")
    product_purl = product.get("@id")
    if not isinstance(product_purl, str) or not product_purl.startswith("pkg:oci/"):
        raise SystemExit("each reviewed OpenVEX product must identify an OCI image")
    packages = product.get("subcomponents")
    if not isinstance(packages, list) or not packages:
        raise SystemExit("each reviewed OpenVEX product must name at least one package")
    direct_products = []
    for package in packages:
        package_purl = package.get("@id") if isinstance(package, dict) else None
        if not isinstance(package_purl, str) or package_purl not in image_packages:
            raise SystemExit("a reviewed OpenVEX package is not in the scanned image")
        direct_products.append({"@id": package_purl})
    statement["products"] = direct_products

Path(output_vex).write_text(json.dumps(document, indent=2) + "\n", encoding="utf-8")
