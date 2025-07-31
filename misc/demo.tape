Output demo.mp4

Set Shell "bash"
Set FontSize 15
Set FontFamily "SauceCodePro Nerd Font"
Set Width 1200
Set Height 700
Set WindowBar Colorful
Set Theme "Solarized Dark Higher Contrast"
Set Padding 20

Type "# 🚀 Uncloud demo: Docker simplicity with multi-machine power"
Enter
Sleep 1s
Type "# Let's deploy a highly available web app with automatic HTTPS in just a couple minutes"
Enter
Enter
Sleep 1s
Type "# First, let's initialise our first machine using a free Oracle Cloud VM"
Enter
Sleep 1s
Type "uc machine init --name oracle-vm ubuntu@152.67.101.197"
Sleep 1s
Enter
# This takes time, so we wait for it to complete.
Wait@1m
Sleep 3s

Enter
Type "# Let's see our machine in the cluster"
Enter
Type "uc machine ls"
Enter
Wait
Sleep 3s

Ctrl+L

Type "# Now let's deploy a simple web app with automatic HTTPS called myapp"
Enter
Sleep 1s
Type "uc run -n myapp -p myapp.psviderski.name:80/https traefik/whoami"
Sleep 1s
Enter
Wait
Sleep 2s

Enter
Type "# The -p flag publishes the container port 80 as HTTPS via reverse proxy Caddy using the provided domain"
Enter
Type "# Let's check it out! First using the auto-generated uncloud domain"
Enter
Sleep 1s

Hide
Sleep 5s
Show

Type "curl https://myapp.xuw3xd.cluster.uncloud.run"
Sleep 1s
Enter
Wait
Sleep 2s

Enter
Type "# For the provided domain to work, we need to manually add a DNS record pointing to the machine IP"
Enter
Type "# To simplify IP management, I added a CNAME record: myapp.psviderski.name -> myapp.xuw3xd.cluster.uncloud.run"
Enter
Sleep 2s
Type "curl https://myapp.psviderski.name"
Sleep 1s
Enter
Wait
Sleep 1s

Enter
Type "# Sweet!"
Enter
Sleep 2s

Ctrl+L

Type "# Let's add redundancy by adding another machine, a Hetzner server in a different region"
Enter
Sleep 1s
Type "uc machine add --name hetzner-server root@5.223.45.199"
Sleep 1s
Enter
Wait@1m
Sleep 3s

Enter
Type "# Now the cool part - let's add a powerful server from my home network to the same cluster"
Enter
Type "# Uncloud lets you mix cloud VMs with your own hardware to save costs on resource-intensive workloads"
Enter
Sleep 2s
Type "uc machine add --name home-server --public-ip none spy@192.168.40.243"
Sleep 2s
Enter
Wait@1m
Sleep 3s

Enter
Type "# Let's see our hybrid cluster spanning cloud providers and home network"
Enter
Sleep 1s
Type "uc machine ls"
Sleep 1s
Enter
Wait
Sleep 3s

Ctrl+L

Type "# Time to scale our service across all machines"
Enter
Sleep 1s
Type "uc scale myapp 3"
Sleep 1s
Enter
Wait
Sleep 1s

Enter
Type "uc inspect myapp"
Sleep 1s
Enter
Wait
Sleep 2s

Enter
Type "# Let's see load balancing in action"
Enter
Sleep 1s
Type "for i in {1..5}; do curl -s https://myapp.psviderski.name | grep Hostname; done"
Sleep 1s
Enter
Wait
Sleep 3s

Ctrl+L

Type "# ✨ And that's it! We've deployed a highly available web app across providers in just a couple minutes"
Enter
Type "# Your service stays up even if the home server and any cloud machine go down"
Enter
Type "# No master nodes to maintain, no complex cluster setup. Try Uncloud today! 🚀"
Enter
Sleep 1s
