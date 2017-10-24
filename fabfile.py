# coding: utf-8
"""
Для обновления сервера необходимо иметь установленный пакет fabric
apt-get install fabric
или
pip install fabric

Запуск обновления:
fab production deploy
Или частями:
fab production pull
fab production restart
"""

from fabric.api import *


@task
def production(branch_name='master', wwwroot='/opt/WWWRoot/go_language_detect/'):
    env.server = 'prod'
    env.remotely = True
    env.hosts = [
        '%s@daria01' % ("user",),
        '%s@daria02' % ("user",),
        '%s@daria03' % ("user",),
        '%s@daria04' % ("user",),
        '%s@daria05' % ("user",),
        '%s@daria06' % ("user",),
    ]
    env.repo = wwwroot
    env.branch = branch_name


def pull():
    """
    Обновляем проект
    """
    require('hosts', 'repo', 'branch')
    with cd(env.repo):
        run("git fetch -p --quiet")
        run("git checkout --quiet {}".format(env.branch))
        run("git pull --quiet || true ")


def build():
    require('hosts', 'repo', 'branch')
    with cd(env.repo):
        with shell_env(GOPATH='/opt/WWWRoot/'):
            run("go get")
            run("go build")

def restart():
    require('hosts', 'repo', 'branch')
    run("supervisorctl update")
    run("supervisorctl restart go_language_detect")

@task
@parallel
def deploy():
    """
    Деплой без миграций
    """
    pull()
    build()
    restart()
