#!/usr/bin/env bash
# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

obs_namespace='open-cluster-management-observability'
deploy_flag=0

sed_command='sed -i-e -e'
if [[ "$(uname)" == "Darwin" ]]; then
    sed_command='sed -i '-e' -e'
fi

usage() {
  cat <<EOF
Usage: $(basename "${BASH_SOURCE[0]}") [-h] [-d | -c] [-n namespace]

Grafana deployment tools.

Available options:

-h, --help        Print this help and exit
-d, --deploy      Deploy all related grafana resources
-c, --clean       Clean all related grafana resources
-n, --namespace   Specify the observability components namespace
EOF
  exit
}

deploy() {
  kubectl get secret -n "$obs_namespace" grafana-config -o 'go-template={{index .data "grafana.ini"}}' | base64 --decode > grafana-dev-config.ini
  if [ $? -ne 0 ]; then
      echo "Failed to get grafana config secret"
      exit 1
  fi
  $sed_command "s~%(domain)s/grafana/$~%(domain)s/grafana-dev/~g" grafana-dev-config.ini
  kubectl create secret generic grafana-dev-config -n "$obs_namespace" --from-file=grafana.ini=grafana-dev-config.ini

  kubectl get deployment -n "$obs_namespace" -l app=multicluster-observability-grafana -o yaml > grafana-dev-deploy.yaml
  if [ $? -ne 0 ]; then
      echo "Failed to get grafana deployment"
      exit 1
  fi
  $sed_command "s~name: grafana$~name: grafana-dev~g" grafana-dev-deploy.yaml
  $sed_command "s~name: observability-grafana$~name: grafana-dev~g" grafana-dev-deploy.yaml
  $sed_command "s~replicas:.*$~replicas: 1~g" grafana-dev-deploy.yaml
  $sed_command "s~grafana-config$~grafana-dev-config~g" grafana-dev-deploy.yaml
  $sed_command "s~app: multicluster-observability-grafana$~app: multicluster-observability-grafana-dev~g" grafana-dev-deploy.yaml
  $sed_command "s~grafana-config$~grafana-dev-config~g" grafana-dev-deploy.yaml
  $sed_command "s~- multicluster-observability-grafana$~- multicluster-observability-grafana-dev~g" grafana-dev-deploy.yaml

  POD_NAME=$(kubectl get pods -n "$obs_namespace" -l app=multicluster-observability-grafana |grep grafana|awk '{split($0, a, " "); print a[1]}' |head -n 1)
  if [ -z "$POD_NAME" ]; then
    echo "Failed to get grafana pod name"
    exit 1
  fi

  GROUP_ID=$(kubectl get pods "$POD_NAME" -n "$obs_namespace" -o jsonpath='{.spec.securityContext.fsGroup}')
  if [[ ${GROUP_ID} == "grafana" ]]; then
    GROUP_ID=472
  fi
  $sed_command "s~  securityContext:.*$~  securityContext: {fsGroup: ${GROUP_ID}}~g" grafana-dev-deploy.yaml
  sed "s~- emptyDir: {}$~- persistentVolumeClaim:$            claimName: grafana-dev~g" grafana-dev-deploy.yaml > grafana-dev-deploy.yaml.bak
  tr $ '\n' < grafana-dev-deploy.yaml.bak > grafana-dev-deploy.yaml
  kubectl apply -f grafana-dev-deploy.yaml

  kubectl get svc -n "$obs_namespace" -l app=multicluster-observability-grafana -o yaml > grafana-dev-svc.yaml
  if [ $? -ne 0 ]; then
      echo "Failed to get grafana service"
      exit 1
  fi
  $sed_command "s~name: grafana$~name: grafana-dev~g" grafana-dev-svc.yaml
  $sed_command "s~app: multicluster-observability-grafana$~app: multicluster-observability-grafana-dev~g" grafana-dev-svc.yaml
  $sed_command "s~clusterIP:.*$~ ~g" grafana-dev-svc.yaml
  # For OCP 4.7, we should remove clusterIPs filed and IPs
  $sed_command "s~clusterIPs:.*$~ ~g" grafana-dev-svc.yaml
  $sed_command 's/\- [0-9]\{1,3\}\.[0-9]\{1,3\}\.[0-9]\{1,3\}\.[0-9]\{1,3\}//g' grafana-dev-svc.yaml
  kubectl apply -f grafana-dev-svc.yaml

  kubectl get ingress -n "$obs_namespace" grafana -o yaml > grafana-dev-ingress.yaml
  if [ $? -ne 0 ]; then
      echo "Failed to get grafana ingress"
      exit 1
  fi
  $sed_command "s~name: grafana$~name: grafana-dev~g" grafana-dev-ingress.yaml
  $sed_command "s~serviceName: grafana$~serviceName: grafana-dev~g" grafana-dev-ingress.yaml
  $sed_command "s~path: /grafana$~path: /grafana-dev~g" grafana-dev-ingress.yaml
  kubectl apply -f grafana-dev-ingress.yaml
  
  cat >grafana-pvc.yaml <<EOL
kind: PersistentVolumeClaim
apiVersion: v1
metadata:
  name: grafana-dev
  namespace: "$obs_namespace"
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  storageClassName: gp2
EOL
  storage_class=$(kubectl get pvc -n "$obs_namespace" | awk '{print $6}'| awk 'NR==2')
  if [ -z "$storage_class" ]; then
      echo "Failed to get storage class"
      exit 1
  fi
  $sed_command "s~gp2$~${storage_class}~g" grafana-pvc.yaml
  kubectl apply -f grafana-pvc.yaml

  # clean all tmp files
  rm -rf grafana-dev-deploy.yaml* grafana-dev-svc.yaml* grafana-dev-ingress.yaml* grafana-dev-config.ini* grafana-pvc.yaml*

  # delete ownerReferences
  kubectl -n "$obs_namespace" patch deployment grafana-dev -p '{"metadata": {"ownerReferences":null}}'
  kubectl -n "$obs_namespace" patch svc grafana-dev -p '{"metadata": {"ownerReferences":null}}'
  kubectl -n "$obs_namespace" patch ingress grafana-dev -p '{"metadata": {"ownerReferences":null}}'
}

clean() {
  kubectl delete secret -n "$obs_namespace" grafana-dev-config
  kubectl delete deployment -n "$obs_namespace" grafana-dev
  kubectl delete svc -n "$obs_namespace" grafana-dev
  kubectl delete ingress -n "$obs_namespace" grafana-dev
  kubectl delete pvc -n "$obs_namespace" grafana-dev
}

msg() {
  echo >&2 -e "${1-}"
}

die() {
  local msg=$1
  local code=${2-1}
  msg "$msg"
  exit "$code"
}

start() {
  if [ $# -eq 0 -o $# -gt 3 ]; then
    usage
  fi

  while [[ $# -gt 0 ]]
  do
  key="$1"
  case $key in
      -h|--help)
      usage
      ;;

      -n|--namespace)
      obs_namespace="$2"
      shift
      shift
      ;;

      -c|--clean)
      clean
      exit 0
      ;;

      -d|--deploy)
      deploy_flag=1
      shift
      ;;

      *)
      usage
      ;;
  esac
  done

  if [ $deploy_flag -eq 1 ]; then
      deploy
      exit
  fi
}

start "$@"
