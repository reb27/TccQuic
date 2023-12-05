#!/usr/bin/env python3

# Uploaded via server_scheduler_test.sh

from mininet.net import Mininet, Host
from mininet.log import setLogLevel
from mininet.util import pmonitor
import os, subprocess
from subprocess import Popen
from utils import HostParams, NetParams, createMininet

class Test():
    def __init__(self, bw: str, server_policy: str):
        print('bw=%d, server_policy=%s' % (bw, server_policy))
        net, server, clients = createMininet(NetParams(clients=[
            HostParams(bw=bw)]))
        self.server_policy: str = server_policy
        self.net: Mininet = net
        self.server: Host = server
        self.client: Host = clients[0]
        self.processes: 'dict[Host, Popen]' = {}

    def run(self):
        try:
            self.__run()
        finally:
            self.__finalize()

    def __run(self):
        print('Starting Mininet')
        self.net.start()
        if not self.net.waitConnected():
            raise RuntimeError('Failed to connect switches')
        setLogLevel('info')

        print('Running test')
        dir = os.path.dirname(os.path.realpath(__file__))

        # Start server
        self.processes[self.server] = self.server.popen(
            [dir + '/main', 'server', self.server_policy],
            cwd=dir,
            stderr=subprocess.STDOUT)
        # Start client
        self.processes[self.client] = self.client.popen(
            [dir + '/main', 'test-client', self.server.IP()],
            cwd=dir,
            stderr=subprocess.STDOUT)
        
        for host, line in pmonitor(self.processes):
            if line:
                if line.endswith('\n'):
                    line = line[:-1]
                print('[%s] %s' % (host, line))
            if len(self.processes) != 2:
                break

    def __finalize(self):
        setLogLevel('warning')
        for process in self.processes.values():
            process.terminate()
        self.net.stop()

##############################################################################

Test(bw=100, server_policy='fifo').run()
