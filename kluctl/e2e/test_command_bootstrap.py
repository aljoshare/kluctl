import os
import shutil
from tempfile import TemporaryDirectory

import pytest

from kluctl import get_kluctl_package_dir
from kluctl.e2e.conftest import assert_readiness, assert_resource_exists, assert_resource_not_exists
from kluctl.e2e.kluctl_test_project import kluctl_project_context
from kluctl.utils.yaml_utils import yaml_save_file, yaml_load_file

dummy_configmap = """
apiVersion: v1
kind: ConfigMap
metadata:
  name: dummy-configmap
  namespace: kube-system
data:
  dummy: value
"""

@pytest.mark.dependency()
def test_command_bootstrap(module_kind_cluster):
    with kluctl_project_context(module_kind_cluster, None) as p:
        p.kluctl("bootstrap", "--yes", "--cluster", p.cluster_name)
        assert_readiness(module_kind_cluster, "kube-system", "Deployment/sealed-secrets", 60 * 5)

@pytest.mark.dependency(depends=["test_command_bootstrap"])
def test_command_bootstrap_upgrade(module_kind_cluster):
    bootstrap_path = os.path.join(get_kluctl_package_dir(), "bootstrap")
    with TemporaryDirectory() as tmpdir:
        shutil.copytree(bootstrap_path, tmpdir, dirs_exist_ok=True)

        with open(os.path.join(tmpdir, "sealed-secrets/dummy.yml"), mode="wt") as f:
            f.write(dummy_configmap)
        k = yaml_load_file(os.path.join(tmpdir, "sealed-secrets/kustomization.yml"))
        k["resources"].append("dummy.yml")
        yaml_save_file(k, os.path.join(tmpdir, "sealed-secrets/kustomization.yml"))

        with kluctl_project_context(module_kind_cluster, tmpdir) as p:
            p.kluctl("bootstrap", "--yes", "--cluster", p.cluster_name)
            assert_resource_exists(module_kind_cluster, "kube-system", "ConfigMap/dummy-configmap")

@pytest.mark.dependency(depends=["test_command_bootstrap_upgrade"])
def test_command_bootstrap_purge(module_kind_cluster):
    with kluctl_project_context(module_kind_cluster, None) as p:
        p.kluctl("bootstrap", "--yes", "--cluster", p.cluster_name)
        assert_resource_not_exists(module_kind_cluster, "kube-system", "ConfigMap/dummy-configmap")