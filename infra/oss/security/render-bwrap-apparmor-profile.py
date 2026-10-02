from __future__ import annotations

import hashlib
import json
from pathlib import Path


ROOT = Path(__file__).resolve().parent
TEMPLATE = ROOT / "moby-v29.2.0-apparmor-template.go"
PROFILE = ROOT / "bwrap-moby-v29.2.0-apparmor.profile"
PROVENANCE = ROOT / "bwrap-moby-v29.2.0-apparmor.provenance.json"
TEMPLATE_SHA256 = "03b143f6a251223c95a5943fd17849c27d7484414e745b0411649b87c19f8c7d"
PROFILE_NAME = "voxis-oss-api-bwrap"


def render_template(source: str) -> str:
    _, rendered = source.split("const baseTemplate = `", 1)
    rendered, _ = rendered.split("`\n", 1)
    rendered = rendered.replace(
        "{{range $value := .Imports}}\n{{$value}}\n{{end}}",
        "#include <tunables/global>",
    )
    rendered = rendered.replace(
        "{{range $value := .InnerImports}}\n  {{$value}}\n{{end}}",
        "  #include <abstractions/base>",
    )
    rendered = rendered.replace("{{.Name}}", PROFILE_NAME)
    rendered = rendered.replace("{{.DaemonProfile}}", "unconfined")
    if "{{" in rendered:
        raise ValueError("unresolved Moby template expression")
    return rendered


def render_profile(template: str) -> str:
    baseline = render_template(template)
    marker = "  deny mount,\n"
    if baseline.count(marker) != 1:
        raise ValueError("Moby mount-deny baseline changed")
    delta = "  mount,\n  pivot_root,\n"
    return baseline.replace(marker, delta)


def provenance(profile: str) -> dict[str, object]:
    return {
        "profile": PROFILE.name,
        "profile_sha256": hashlib.sha256(profile.encode("utf-8")).hexdigest(),
        "base_profile": {
            "docker_engine_version": "29.2.0",
            "docker_engine_commit": "9c62384",
            "source": "https://raw.githubusercontent.com/moby/moby/docker-v29.2.0/vendor/github.com/moby/profiles/apparmor/template.go",
            "sha256": TEMPLATE_SHA256,
            "copyright": "Copyright The Moby Authors",
            "license": "Apache-2.0",
            "license_file": "LICENSE.moby",
            "license_sha256": "7c87873291f289713ac5df48b1f2010eb6963752bbd6b530416ab99fc37914a8",
        },
        "rendered_placeholders": {"Name": PROFILE_NAME, "DaemonProfile": "unconfined"},
        "only_policy_delta": {"removed_rule": "deny mount,", "added_rules": ["mount,", "pivot_root,"]},
        "tradeoff": "The outer API process starts without capabilities. Bubblewrap creates a nested user and mount namespace for its mounts. AppArmor 3.0.8 could not express Bubblewrap's dynamic bind and remount sequence as a reliable narrow rule set. This profile preserves every other Docker default rule but permits mount and pivot-root mediation for the API container.",
        "required_outer_constraints": [
            "uid 10001",
            "read-only root filesystem",
            "no-new-privileges",
            "all OCI capabilities dropped",
            "pinned Docker seccomp profile",
        ],
        "scratch_storage": "The Compose bind mount must be provisioned by the operator. Compose does not claim or enforce noexec, nosuid, or nodev mount options for it.",
    }


def main() -> None:
    template = TEMPLATE.read_text(encoding="utf-8")
    actual_template_hash = hashlib.sha256(template.encode("utf-8")).hexdigest()
    if actual_template_hash != TEMPLATE_SHA256:
        raise SystemExit("unexpected Moby AppArmor template hash")
    rendered = render_profile(template)
    expected = PROFILE.read_text(encoding="utf-8") if PROFILE.exists() else rendered
    if expected != rendered:
        raise SystemExit("AppArmor profile differs from the mechanical render")
    PROFILE.write_text(rendered, encoding="utf-8", newline="\n")
    PROVENANCE.write_text(json.dumps(provenance(rendered), indent=2) + "\n", encoding="utf-8", newline="\n")


if __name__ == "__main__":
    main()
