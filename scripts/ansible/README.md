# Ansible for AvalancheGo

[Ansible](https://ansible.com) playbooks, roles, & inventories to install
[AvalancheGo](https://github.com/ava-labs/avalanchego) as a systemd service.
Target(s) can be

- localhost
- Cloud VMs (e.g. Amazon, Azure, Digital Ocean)
- Raspberry Pi
- any host running a supported operating system
- any combination of the above


## Using

To create an AvalancheGo service on localhost

1. Check you have Ansible 2.9+ (see [Installing](#installing))
2. Clone the AvalancheGo git repository
    ```
    $ git clone https://github.com/ava-labs/avalanchego
    ```

3. Change to this directory
    ```
    $ cd avalanchego/scripts/ansible
    ```

4. Run the service playbook
    ```
    $ ansible-playbook \
        -i inventories/localhost.yml \
        --ask-become-pass \
        service_playbook.yml
    ```

   You don't need `--ask-become-pass` if your account doesn't require a sudo
   password. To install on remote hosts you will need to create an inventory,
   see [customising].

   Output should look similar to the [example run](#example-run).

5. Check the service is running
    ```
    $ systemctl status avalanchego
    ```

    The output should look similar to
    ```
    ● avalanchego.service - AvalancheGo node for Avalanche consensus network
    Loaded: loaded (/etc/systemd/system/avalanchego.service; enabled; vendor preset: enabled)
    Active: active (running) since Wed 2020-10-21 10:00:00 UTC; 32s ago
    ...
    ```


## Installing

Ansible 2.9 (or higher) is required. To check, run

```
ansible --version
```

the first line of output should be like `ansible 2.9.x`, or `ansible 2.10.x`
(`x` can be any number). If the output includes `ansible: command not found`,
or an earlier version (e.g. `ansible 2.8.16`), then you need to install a
supported version.

To install a supported version

1. Create a Python Virtualenv
    ```
    $ python3 -m venv venv/
    ```

2. Activate the Virtualenv
    ```
    $ source venv/bin/activate
    ```

4. Install Ansible inside the virtualenv
    ```
    $ pip install "ansible>=2.9"
    ```


## Customising

To run against remote targets you'll need an [Ansible inventory](https://docs.ansible.com/ansible/latest/user_guide/intro_inventory.html#inventory-basics-formats-hosts-and-groups).
Here are some examples to use as a starting point.

### Amazon

```yaml
avalanche_nodes:
  hosts:
    ec2-203-0-113-42.us-east-1.compute.amazonaws.com:
    ec2-203-0-113-9.ap-southeast-1.compute.amazonaws.com:
  vars:
    ansible_ssh_private_key_file: "~/.ssh/aws_identity.pem"
    ansible_user: "ubuntu"
```

### Raspberry Pi

```yaml
avalanche_nodes:
  hosts:
    raspberrypi.local:
  vars:
    ansible_user: "pi"
```

## Requirements

Target operating systems supported by these roles & playbooks are

- CentOS 7
- CentOS 8
- Debian 10
- Raspberry Pi OS
- Ubuntu 18.04 LTS
- Ubuntu 20.04 LTS


## Example run

```
PLAY [Configure Avalanche service] ****************************************************

TASK [Gathering Facts] ****************************************************************
ok: [localhost]

TASK [golang_base : Dispatch tasks] ***************************************************
included: …/roles/golang_base/tasks/ubuntu-20.04.yml for localhost

TASK [golang_base : Install Go] *******************************************************
ok: [localhost]

TASK [avalanche_base : Dispatch tasks] ************************************************
included: …/roles/avalanche_base/tasks/ubuntu.yml for localhost

TASK [avalanche_base : Install Avalanche dependencies] ********************************
ok: [localhost]

TASK [avalanche_build : Update git clone] *********************************************
changed: [localhost]

TASK [avalanche_build : Build project] ************************************************
changed: [localhost]

TASK [avalanche_user : Create Avalanche daemon group] *********************************
changed: [localhost]

TASK [avalanche_user : Create Avalanche daemon user] **********************************
[WARNING]: The value False (type bool) in a string field was converted to 'False'
(type string). If this does not look like what you expect, quote the entire value to
ensure it does not change.
changed: [localhost]

TASK [avalanche_install : Create shared directories] **********************************
ok: [localhost] => (item={'path': '/var/local/lib'})
changed: [localhost] => (item={'path': '/var/local/log'})

TASK [avalanche_install : Create Avalanche directories] *******************************
ok: [localhost] => (item=/var/local/lib/avalanchego)
changed: [localhost] => (item=/var/local/lib/avalanchego/db)
changed: [localhost] => (item=/var/local/lib/avalanchego/staking)
changed: [localhost] => (item=/var/local/log/avalanchego)
changed: [localhost] => (item=/usr/local/lib/avalanchego)

TASK [avalanche_install : Install Avalanche binary] ***********************************
changed: [localhost]

TASK [avalanche_install : Install Avalanche plugins] **********************************
changed: [localhost] => (item={'path': '~auser/go/src/github.com/ava-labs/avalanchego/build/plugins/evm'})

TASK [avalanche_staker : Create staking key] ******************************************
changed: [localhost]

TASK [avalanche_staker : Create staking certificate signing request] ******************
changed: [localhost]

TASK [avalanche_staker : Create staking certificate] **********************************
changed: [localhost]

TASK [avalanche_service : Configure Avalanche service] ********************************
changed: [localhost]

TASK [avalanche_service : Enable Avalanche service] ***********************************
changed: [localhost]

RUNNING HANDLER [avalanche_service : Reload systemd] **********************************
ok: [localhost]

RUNNING HANDLER [avalanche_service : Restart Avalanche service] ***********************
changed: [localhost]

PLAY RECAP ****************************************************************************
localhost : ok=20  changed=14  unreachable=0  failed=0  skipped=0  rescued=0  ignored=0
```
