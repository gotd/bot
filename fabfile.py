from fabric import task, Connection


@task
def deploy(c):
    """
    Deploy bot to production

    :type c: Connection
    """
    c.put('./bot', '/tmp/bot')
    c.run('mv /tmp/bot /home/bot/bot')

    c.run('systemctl restart bot')
    c.run('tg send -p gotd_ru "New version is deployed"')

