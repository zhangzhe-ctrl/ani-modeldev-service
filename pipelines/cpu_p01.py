"""The single CPU-P01 KFP assembly entry; compile on Fedora only."""
import argparse
import json
import re

from kfp import compiler, dsl, kubernetes


def compile_pipeline(configuration, output):
    required = {"component_image", "owner_config_map", "storage_credentials_secret",
                "storage_class", "workspace_size", "workspace_access_mode"}
    if set(configuration) != required:
        raise ValueError("all frozen deployment bindings are required")
    if not re.fullmatch(r"[^\s@]+@sha256:[0-9a-f]{64}", configuration["component_image"]):
        raise ValueError("component image must use a manifest digest")
    for key in ("owner_config_map", "storage_credentials_secret", "storage_class"):
        if not re.fullmatch(r"[a-z0-9](?:[-a-z0-9.]{0,251}[a-z0-9])?", configuration[key]):
            raise ValueError("invalid frozen Kubernetes binding")
    if not re.fullmatch(r"[1-9][0-9]*(?:Mi|Gi)", configuration["workspace_size"]):
        raise ValueError("explicit positive workspace capacity is required")
    if configuration["workspace_access_mode"] not in ("ReadWriteOnce", "ReadWriteMany"):
        raise ValueError("workspace must support sequential component and Trainer mounts")

    def container(step, execution_id, spec_hash, extra=()):
        return dsl.ContainerSpec(
            image=configuration["component_image"],
            command=["/ani-modeldev-step", step],
            args=["--config", "/etc/modeldev-step/config.json",
                  "--execution-id", execution_id, "--spec-hash", spec_hash,
                  "--run-id", dsl.PIPELINE_JOB_ID_PLACEHOLDER,
                  "--task-id", dsl.PIPELINE_TASK_ID_PLACEHOLDER, *extra],
        )

    @dsl.container_component
    def prepare(execution_id: str, spec_hash: str, pvc_name: str):
        return container("prepare", execution_id, spec_hash, ["--pvc-name", pvc_name])

    @dsl.container_component
    def train_wait(execution_id: str, spec_hash: str):
        return container("train-wait", execution_id, spec_hash)

    @dsl.container_component
    def collect(execution_id: str, spec_hash: str):
        return container("collect", execution_id, spec_hash)

    @dsl.container_component
    def publish(execution_id: str, spec_hash: str, candidate: dsl.OutputPath(str)):
        return container("publish", execution_id, spec_hash, ["--candidate-file", candidate])

    @dsl.container_component
    def close(execution_id: str, spec_hash: str, candidate: str = ""):
        return container("close", execution_id, spec_hash, ["--candidate-json", candidate])

    def configure(task, *, storage=False, timeout=600):
        task.set_caching_options(False).set_retry(0)
        task.set_cpu_request("100m").set_cpu_limit("500m")
        task.set_memory_request("128Mi").set_memory_limit("512Mi")
        kubernetes.set_timeout(task, timeout)
        kubernetes.set_security_context(task, run_as_user=10001, run_as_group=10001, run_as_non_root=True)
        kubernetes.use_config_map_as_volume(task, configuration["owner_config_map"], "/etc/modeldev-step")
        for name, path in (("ANI_POD_NAME", "metadata.name"), ("ANI_POD_UID", "metadata.uid"),
                           ("ANI_POD_NAMESPACE", "metadata.namespace")):
            kubernetes.use_field_path_as_env(task, name, path)
        kubernetes.empty_dir_mount(task, "component-tmp", "/tmp", size_limit="16Mi")
        if storage:
            kubernetes.use_secret_as_volume(task, configuration["storage_credentials_secret"], "/var/run/modeldev-storage")
        return task

    @dsl.pipeline(name="general-cpu")
    def pipeline(execution_id: str, spec_hash: str):
        # This official KFP resource primitive is interpreted by its backend;
        # argostub/createpvc is not an image that this application runs.
        workspace = kubernetes.CreatePVC(
            pvc_name_suffix="workspace", access_modes=[configuration["workspace_access_mode"]],
            size=configuration["workspace_size"], storage_class_name=configuration["storage_class"],
        ).set_caching_options(False).set_retry(0)
        prepared = configure(prepare(execution_id=execution_id, spec_hash=spec_hash, pvc_name=workspace.output), storage=True)
        trained = configure(train_wait(execution_id=execution_id, spec_hash=spec_hash).after(prepared), timeout=1800)
        collected = configure(collect(execution_id=execution_id, spec_hash=spec_hash).after(trained))
        published = configure(publish(execution_id=execution_id, spec_hash=spec_hash).after(collected), storage=True)
        for task in (prepared, collected, published):
            kubernetes.mount_pvc(task, workspace.output, "/workspace")
        configure(close(execution_id=execution_id, spec_hash=spec_hash, candidate=published.output)
                  .after(workspace, prepared, trained, collected, published).ignore_upstream_failure())
        # No DeletePVC: publication failure must retain the unique outputs.

    compiler.Compiler().compile(pipeline_func=pipeline, package_path=output)


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--config", required=True)
    parser.add_argument("--output", required=True)
    options = parser.parse_args()
    with open(options.config, encoding="utf-8") as source:
        compile_pipeline(json.load(source), options.output)
