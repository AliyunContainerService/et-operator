#!/bin/bash
file="$0"
filePath=$(cd $(dirname "$0"); pwd)
hostfile='/etc/et/hostfile'
show_usage="[Usage]: ${filepath}/${file} --delete or --add [pod1:1,pod2:1,...]"

function addWorkers(){
  echo "will add pods: ${pods} to ${hostfile}"
  podsArray=(${pods//,/ })
  podsLines=""
  num=${#podsArray[@]}
  for ((i=0;i<num;i++))
  do
    if (( i != num-1)); then
      grep -q "^${podsArray[i]}" ${hostfile} || podsLines+=${podsArray[i]}"\n"
    else
      grep -q "^${podsArray[i]}" ${hostfile} || podsLines+=${podsArray[i]}
    fi
  done
  echo "$podsLines"
  echo -e "$podsLines" >> ${hostfile}

}

function deleteWorkers(){
  echo "will delete pods: ${pods} from ${hostfile}"
  podsArray=(${pods//,/ })

  sed_cmd="sed -i "
  for pod in ${podsArray[@]}
  do
    echo "delete ${pod} from ${hostfile}"
    pattern="/^${pod}.*/d "
    sed_cmd+=" -e "$pattern
  done
  sed_cmd+=" "${hostfile}
  $sed_cmd
  sleep 5s

:<<BLOCK
  for pod in ${podsArray[@]}
  do
    echo "checking callback when deleting ${pod} from ${hostfile}"
    ssh -o PasswordAuthentication=no -o StrictHostKeyChecking=no ${pod%:*} "export PERSEUS_WITH_K8S=1 && python3 -c 'import perseus.tensorflow.horovod as hvd; hvd.check_callback_file(0, 0)'";
  done
BLOCK
}

function main(){
  while [ -n "$1" ]
  do
    case "$1" in
      --add) pods="$2";
             addWorkers;
             shift ;;
      --delete) pods="$2";
             deleteWorkers;
             shift ;;
      *) echo "params: [$1] is not an option";
             echo "${show_usage}";
             exit 1 ;;  # 发现未知参数，直接退出
    esac
    shift
  done

}

main "$@"