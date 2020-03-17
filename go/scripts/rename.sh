cp `dirname $0`/../bls/bls_* .
for name in bls_*
do
    newname=$1"$(echo "$name" | cut -c4-)"
    mv "$name" "$newname"
    sed -i "" "s/package bls/package $1/g" $newname
done

