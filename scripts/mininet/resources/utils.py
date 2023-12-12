from mininet.topo import Topo
from mininet.link import TCLink
from mininet.net import Mininet, Host

class HostParams:
    def __init__(self, bw = None, delay = None, loss = None):
        self.bw = bw
        self.delay = delay
        self.loss = loss

class NetParams:
    def __init__(self, server: HostParams = HostParams(),
                 clients: 'list[HostParams]' = []):
        self.server = server
        self.clients = clients

class _TestTopo(Topo):
    def build(self, params: NetParams):
        switch = self.addSwitch('s0')

        host = self.addHost('h0')
        self.addLink(host, switch, cls=TCLink, bw=params.server.bw,
                     delay=params.server.delay, loss=params.server.loss)

        for i in range(len(params.clients)):
            host = self.addHost('h%s' % (i + 1))
            self.addLink(host, switch, cls=TCLink, bw=params.clients[i].bw,
                         delay=params.clients[i].delay,
                         loss=params.clients[i].loss)

'''
Returns (mininet, server, clients)
'''
def createMininet(params: NetParams) -> (Mininet, Host, 'list[Host]'):
    mininet = Mininet(_TestTopo(params))
    server = mininet.hosts[0]
    clients = mininet.hosts[1:]
    return mininet, server, clients
