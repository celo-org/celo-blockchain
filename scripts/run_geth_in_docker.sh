# allow exiting using CTRL + C
exit_func() {
        echo "Exiting..."
        exit 1
}
trap exit_func SIGTERM SIGINT

# 150 is an arbitrary choice, which seems good enough
stty cols 150
sleep 1
geth "$@"
