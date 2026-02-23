#!/bin/bash
BASE=/Users/martinsuchenak/Devel/projects/phantom
for i in {1..5}; do
  MNT=/tmp/test-mnt-race$i
  UPPER=/tmp/test-upper-race$i
  mkdir -p $MNT $UPPER
  unionfs -o allow_other,use_ino,suid,dev $UPPER=RW:$BASE=RO $MNT
  if ! [ -d $MNT/.git ]; then
    echo "Iteration $i: .git NOT FOUND!"
  else
    echo "Iteration $i: OK"
  fi
done
# Cleanup
for i in {1..5}; do
  MNT=/tmp/test-mnt-race$i
  UPPER=/tmp/test-upper-race$i
  umount $MNT || diskutil unmount force $MNT
  rm -rf $MNT $UPPER
done
echo "Done"
