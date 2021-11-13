# docker-dns

一个用于根据docker容器name、id、network alias到IP地址对应的小工具，可作为DNS服务器运行

## CLI运行

./docker-dns [container name]

## 作为DNS服务器

./docker-dns -d -s [domain suffix]

域名后缀默认为d.com，比如有容器id:f4505a5420c9，查询`f4505a5420c9.d.com`即可得到容器的ip，比如：

dig @127.0.0.1 -p5300 f4505a5420c9.d.com

## 作为系统DNS服务

### systemd-resolved

systemd-resolved提供一种叫"routing domains"（路由域）的功能，即匹配一个特定域名后缀，然后向特定DNS服务器发送查询请求。根据此功能，我们可以在docker0接口上设置一个路由域"~d.com"，然后指定DNS服务器，就可以提供系统DNS服务

```sh
docker build . -t docker-dns
docker run -it --rm -v /run/docker.sock:/run/docker.sock -p 5300:5300/udp docker-dns
sudo resolvectl domain docker0 "~d.com"
sudo resolvectl dns docker0 127.0.0.1:5300
```

注意，这里必须要求docker中有容器在运行，即docker0接口在工作状态，这套方案才能正常运行。当然你也可以配置到别的在接口上。我目前没有发现systemd-resolved有更好的设置方法，如果你有更好的配置方案，欢迎issue中讨论一下。
