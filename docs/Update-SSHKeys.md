# Updating SSH Keys with the MCD

By default, the OpenShift 4.0 installer creates a single user named `core` (derived in spirit from CoreOS Container Linux) with optional SSH keys specified at install time.

This operator supports updating the SSH keys of user `core` via a MachineConfig object. The SSH keys are updated for all members of the MachineConfig pool specified in the MachineConfig, for example: all worker nodes.

Please note that RHCOS nodes will be [annotated](https://github.com/openshift/machine-config-operator/blob/master/docs/MachineConfigDaemon.md#annotating-on-ssh-access) when accessed via SSH.

## Unsupported Operations

- The MCD will not add any new users.

- The MCD will not delete the user `core`.

- The MCD will not make any changes to any other User fields for user `core` other than `sshAuthorizedKeys`.

## Info you will need

You will need the following information for the MachineConfig that will be used to update your SSHKeys.

- `machineconfiguration.openshift.io/role:` the MachineConfig that is created will be applied to all nodes with the role specified here. For example: `master` or `worker`

- `name:` each MachineConfig that you create must have a unique name. Do not reuse the same MachineConfig name. MachineConfigs are cumulative and applied in alphabetical/lexicographic order so that the last MachineConfig will be the final one applied. We recommend using a naming scheme that accounts for this such as: `ssh-workers-01`, `ssh-workers-02`, `ssh-master-01`, `ssh-masters-02`, etc...

- `sshAuthorizedKeys:` you will need one or more public keys to be assigned to user `core`.  Multiple SSH Keys should begin on different lines and each be preceded by `-`.

## Example MachineConfig (with 2 SSH Keys added)
 ```yaml
# example-ssh-update.yaml
apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
  labels:
    machineconfiguration.openshift.io/role: worker
  name: ssh-workers-01
spec:
  config:
    passwd:
      users:
      - name: core
        sshAuthorizedKeys:
        - ssh-rsa ABC123....
        - ssh-ed25519 XYZ7890....
        - ecdsa-sha2-nistp256 AAAAE2....

 ```
 ## Common MachineConfig Pitfalls
 - Assuming that the name of the file is the name of the MachineConfig: If you choose to modify one of your existing MachineConfigs, do not forget to change the `metadata: name:` field.

 - Thinking that you will retain an old SSH key when you apply an SSH update: New SSH updates completely overwrite existing keys. If you would like to add an additional SSH key and retain the current SSH Key, you must add *both* the old and new SSH keys into the new MachineConfig.

 - Updating `user: name`: Do not update the `user: name` field. The only user currently supported is `core` as shown in the above example config.

 ## Applying the MachineConfig

Now with your new MachineConfig yaml file (using the example above):
```sh
    oc create -f example-ssh-update.yaml
```

You should see the new MachineConfig name appear almost immediately, from our example config:
```sh
    oc get machineconfigs

    NAME                                                        GENERATEDBYCONTROLLER              IGNITIONVERSION   CREATED
    00-master                                                   4.0.0-alpha.0-83-g0a84145f-dirty   2.2.0             87m
    00-master-ssh                                               4.0.0-alpha.0-83-g0a84145f-dirty   2.2.0             87m
    00-worker                                                   4.0.0-alpha.0-83-g0a84145f-dirty   2.2.0             87m
    00-worker-ssh                                               4.0.0-alpha.0-83-g0a84145f-dirty   2.2.0             87m
    01-master-container-runtime                                 4.0.0-alpha.0-83-g0a84145f-dirty   2.2.0             87m
    01-master-kubelet                                           4.0.0-alpha.0-83-g0a84145f-dirty   2.2.0             87m
    01-worker-container-runtime                                 4.0.0-alpha.0-83-g0a84145f-dirty   2.2.0             87m
    01-worker-kubelet                                           4.0.0-alpha.0-83-g0a84145f-dirty   2.2.0             87m
    99-master-9c65f9fb-41d0-11e9-994d-02360d172130-registries   4.0.0-alpha.0-83-g0a84145f-dirty   2.2.0             84m
    99-worker-9c67b221-41d0-11e9-994d-02360d172130-registries   4.0.0-alpha.0-83-g0a84145f-dirty   2.2.0             84m
    rendered-master-a1884339a91f02f19898fe9c5929256b            4.0.0-alpha.0-83-g0a84145f-dirty   2.2.0             87m
    rendered-worker-777a036887bb25188f47c6d3219eabb9            4.0.0-alpha.0-83-g0a84145f-dirty   2.2.0             2s
    rendered-worker-e99b9948bccfd387c71397b31e2c985e            4.0.0-alpha.0-83-g0a84145f-dirty   2.2.0             87m
    ssh-workers-01                                                                                                   7s
```

You are then able to monitor the MCD logs of a worker or master (whichever the config applied to), which should check the proposed changes and reboot into the new config:
```sh
   oc logs -f -n openshift-machine-config-operator machine-config-daemon-<hash>
```
If the update was succesfully applied, you should expect to see lines similar to in these logs:
```sh
   I0111 19:59:07.360110    7993 update.go:258] SSH Keys reconcilable
   ...
   I0111 19:59:07.371253    7993 update.go:569] Writing SSHKeys at "/home/core/.ssh"
   ...
   I0111 19:59:07.372208    7993 update.go:613] machine-config-daemon initiating reboot: Node will reboot into config worker-96b48815fa067f651fa50541ea6a9b5d
```
After the node reboots, expect to see the daemons for the node specified restarted:

```sh
    oc get pods -n openshift-machine-config-operator

    NAME                                         READY     STATUS    RESTARTS   AGE
    machine-config-controller-68f5989588-2cfvq   1/1       Running   0          1h
    machine-config-daemon-58d6c                  1/1       Running   0          1h
    machine-config-daemon-c7jkk                  1/1       Running   1          1h
    machine-config-daemon-ddsnp                  1/1       Running   1          1h
    machine-config-daemon-kx49n                  1/1       Running   1          1h
    machine-config-daemon-q8d7j                  1/1       Running   0          1h
    machine-config-daemon-w68t9                  1/1       Running   0          1h
    machine-config-operator-769967ddf5-9blb8     1/1       Running   0          1h
    machine-config-server-7gckv                  1/1       Running   0          1h
    machine-config-server-98cpz                  1/1       Running   0          1h
    machine-config-server-pzj68                  1/1       Running   0          1h
```

If we check the same daemon's logs, we should now see similar lines in the output:

```sh
    oc logs -f -n openshift-machine-config-operator machine-config-daemon-<same-hash>

    ...
    I0111 20:00:15.755052    6900 daemon.go:497] Completing pending config worker-52df682dc5cb3976b063ef3f197ead5e
    ...
    I0111 20:00:15.769349    6900 update.go:613] machine-config-daemon: completed update for config worker-52df682dc5cb3976b063ef3f197ead5e
    ...
    I0111 20:00:15.778909    6900 daemon.go:503] In desired config worker-52df682dc5cb3976b063ef3f197ead5e
```


