# Prevent error related to running geth without a tty
# 150 is an arbitrary choice, which seems good enough
stty cols 150
sleep 1
exec geth "$@"
