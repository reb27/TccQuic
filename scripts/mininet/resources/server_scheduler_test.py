#!/usr/bin/env python3

# Uploaded via server_scheduler_test.sh

from mininet.net import Mininet, Host
from mininet.log import setLogLevel
import os, time, subprocess
from subprocess import Popen
from utils import HostParams, NetParams, createMininet

###########################################################################

# Test parameters
NET_PARAMS = NetParams(
    server = HostParams(bw=100),
    clients = [
        HostParams(bw=100, delay='1ms'),
        # HostParams(bw=100, delay='1ms'),
        # HostParams(bw=100, delay='1ms'),
        # HostParams(bw=100, delay='1ms'),
        # HostParams(bw=100, delay='1ms'),
    ],
)
SERVER_POLICY = 'fifo'

###########################################################################

class Test():
    def __init__(self):
        self.net: Mininet
        self.server: Host
        self.clients: 'list[Host]'
        self.net, self.server, self.clients = createMininet(NET_PARAMS)
        self.processes: 'set[Popen]' = set()

    def setup(self) :
        print('Starting Mininet')
        self.net.start()
        if not self.net.waitConnected():
            raise RuntimeError('Failed to connect switches')
        setLogLevel('info')

    def run(self):
        print('Running test')
        dir = os.path.dirname(os.path.realpath(__file__))

        # Start server
        processServer = self.server.popen(
            [dir + '/main', 'server', SERVER_POLICY, self.server.IP()],
            stdout=subprocess.PIPE)
        self.processes.add(processServer)
        # Start client
        self.clients[0].cmdPrint(
            [dir + '/main', 'test-client', self.server.IP()])

    def finalize(self):
        setLogLevel('warning')
        for process in self.processes:
            process.terminate()
        self.net.stop()

test = Test()
try:
    test.setup()
    test.run()
finally:
    test.finalize()
