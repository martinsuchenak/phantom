#!/bin/bash
BASE=/Users/martinsuchenak/Devel/projects/phantom
MNT=/tmp/test-mnt-race
UPPER=/tmp/test-upper-race
mkdir -p $MNT $UPPER
for i in {1..10}; do
  unionfs -o allow_other,use_ino,suid,dev $UPPER=RW:$BASE=RO $MNT
  if ! [ -d $MNT/.git ]; then
    echo "Iteration $i: .git NOT FOUND!"
  fi
  umount $MNT || diskutil unmount force $MNT
done
rm -rf $MNT $UPPER
echo "Done"
