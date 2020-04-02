cp `dirname $0`/../bls/bls_* .
for name in `ls bls_* | grep -v _test`
do
    echo $name
    newname=$1"$(echo "$name" | cut -c4-)"
    mv "$name" "$newname"
    sed -i "" "s/package bls/package $1/g" $newname
done
