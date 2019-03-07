// mgo - MongoDB driver for Go
//
// Copyright (c) 2019 - Canonical Ltd
//
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:
//
// 1. Redistributions of source code must retain the above copyright notice, this
//    list of conditions and the following disclaimer.
// 2. Redistributions in binary form must reproduce the above copyright notice,
//    this list of conditions and the following disclaimer in the documentation
//    and/or other materials provided with the distribution.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
// ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
// WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
// DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT OWNER OR CONTRIBUTORS BE LIABLE FOR
// ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES
// (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES;
// LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND
// ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS
// SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package mgo_test

import (
	"sync"

	. "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) setupTxnSession(c *C) *mgo.Session {
	// get the test infrastructure ready for doing transactions.
	if !s.versionAtLeast(4, 0) {
		c.Skip("transactions not supported before 4.0")
	}
	session, err := mgo.Dial("localhost:40011")
	c.Assert(err, IsNil)
	return session
}

func (s *S) setup2Sessions(c *C) (*mgo.Session, *mgo.Collection, *mgo.Session, *mgo.Collection) {
	session1 := s.setupTxnSession(c)
	// Collections must be created outside of a transaction
	coll1 := session1.DB("mydb").C("mycoll")
	err := coll1.Create(&mgo.CollectionInfo{})
	if err != nil {
		session1.Close()
		c.Assert(err, IsNil)
	}
	session2 := session1.Copy()
	coll2 := session2.DB("mydb").C("mycoll")
	return session1, coll1, session2, coll2
}

func (s *S) TestTransactionInsertCommitted(c *C) {
	session1, coll1, session2, coll2 := s.setup2Sessions(c)
	defer session1.Close()
	defer session2.Close()
	c.Assert(session1.StartTransaction(), IsNil)
	// call Abort in case there is a problem, but ignore an error if it was committed,
	// otherwise the server will block in DropCollection because the transaction is active.
	defer session1.AbortTransaction()
	c.Assert(coll1.Insert(bson.M{"a": "a", "b": "b"}), IsNil)
	var res bson.M
	// Should be visible in the session that has the transaction
	c.Assert(coll1.Find(bson.M{"a": "a"}).Select(bson.M{"a": 1, "b": 1, "_id": 0}).One(&res), IsNil)
	c.Check(res, DeepEquals, bson.M{"a": "a", "b": "b"})
	// Since the change was made in a transaction, session 2 should not see the document
	err := coll2.Find(bson.M{"a": "a"}).One(&res)
	c.Check(err, Equals, mgo.ErrNotFound)
	c.Assert(session1.CommitTransaction(), IsNil)
	// Now that it is committed, session2 should see it
	err = coll2.Find(bson.M{"a": "a"}).Select(bson.M{"a": 1, "b": 1, "_id": 0}).One(&res)
	c.Check(err, IsNil)
	c.Check(res, DeepEquals, bson.M{"a": "a", "b": "b"})
}

func (s *S) TestTransactionInsertAborted(c *C) {
	session1, coll1, session2, coll2 := s.setup2Sessions(c)
	defer session1.Close()
	defer session2.Close()
	c.Assert(session1.StartTransaction(), IsNil)
	// call Abort in case there is a problem, but ignore an error if it was committed,
	// otherwise the server will block in DropCollection because the transaction is active.
	defer session1.AbortTransaction()
	c.Assert(coll1.Insert(bson.M{"a": "a", "b": "b"}), IsNil)
	var res bson.M
	// Should be visible in the session that has the transaction
	c.Assert(coll1.Find(bson.M{"a": "a"}).Select(bson.M{"a": 1, "b": 1, "_id": 0}).One(&res), IsNil)
	c.Check(res, DeepEquals, bson.M{"a": "a", "b": "b"})
	// Since the change was made in a transaction, session 2 should not see the document
	err := coll2.Find(bson.M{"a": "a"}).One(&res)
	c.Check(err, Equals, mgo.ErrNotFound)
	c.Assert(session1.AbortTransaction(), IsNil)
	// Since it is Aborted, nobody should see the object
	err = coll2.Find(bson.M{"a": "a"}).One(&res)
	c.Check(err, Equals, mgo.ErrNotFound)
	err = coll1.Find(bson.M{"a": "a"}).One(&res)
	c.Check(err, Equals, mgo.ErrNotFound)

}

func (s *S) TestTransactionUpdateCommitted(c *C) {
	session1, coll1, session2, coll2 := s.setup2Sessions(c)
	defer session1.Close()
	defer session2.Close()
	c.Assert(coll1.Insert(bson.M{"a": "a", "b": "b"}), IsNil)
	c.Assert(session1.StartTransaction(), IsNil)
	// call Abort in case there is a problem, but ignore an error if it was committed,
	// otherwise the server will block in DropCollection because the transaction is active.
	defer session1.AbortTransaction()
	c.Assert(coll1.Update(bson.M{"a": "a"}, bson.M{"$set": bson.M{"b": "c"}}), IsNil)
	// Should be visible in the session that has the transaction
	var res bson.M
	c.Assert(coll1.Find(bson.M{"a": "a"}).Select(bson.M{"a": 1, "b": 1, "_id": 0}).One(&res), IsNil)
	c.Check(res, DeepEquals, bson.M{"a": "a", "b": "c"})
	// Since the change was made in a transaction, session 2 should not see it
	c.Assert(coll2.Find(bson.M{"a": "a"}).Select(bson.M{"a": 1, "b": 1, "_id": 0}).One(&res), IsNil)
	c.Check(res, DeepEquals, bson.M{"a": "a", "b": "b"})
	c.Assert(session1.CommitTransaction(), IsNil)
	// Now that it is committed, session2 should see it
	c.Assert(coll2.Find(bson.M{"a": "a"}).Select(bson.M{"a": 1, "b": 1, "_id": 0}).One(&res), IsNil)
	c.Check(res, DeepEquals, bson.M{"a": "a", "b": "c"})
}

func (s *S) TestTransactionUpdateAllCommitted(c *C) {
	session1, coll1, session2, coll2 := s.setup2Sessions(c)
	defer session1.Close()
	defer session2.Close()
	c.Assert(coll1.Insert(bson.M{"a": "a", "b": "b"}), IsNil)
	c.Assert(coll1.Insert(bson.M{"a": "2", "b": "b"}), IsNil)
	c.Assert(session1.StartTransaction(), IsNil)
	// call Abort in case there is a problem, but ignore an error if it was committed,
	// otherwise the server will block in DropCollection because the transaction is active.
	defer session1.AbortTransaction()
	changeInfo, err := coll1.UpdateAll(nil, bson.M{"$set": bson.M{"b": "c"}})
	c.Assert(err, IsNil)
	c.Check(changeInfo.Matched, Equals, 2)
	c.Check(changeInfo.Updated, Equals, 2)
	// Should be visible in the session that has the transaction
	var res bson.M
	c.Assert(coll1.Find(bson.M{"a": "a"}).Select(bson.M{"a": 1, "b": 1, "_id": 0}).One(&res), IsNil)
	c.Check(res, DeepEquals, bson.M{"a": "a", "b": "c"})
	c.Assert(coll1.Find(bson.M{"a": "2"}).Select(bson.M{"a": 1, "b": 1, "_id": 0}).One(&res), IsNil)
	c.Check(res, DeepEquals, bson.M{"a": "2", "b": "c"})
	// Since the change was made in a transaction, session 2 should not see it
	c.Assert(coll2.Find(bson.M{"a": "a"}).Select(bson.M{"a": 1, "b": 1, "_id": 0}).One(&res), IsNil)
	c.Check(res, DeepEquals, bson.M{"a": "a", "b": "b"})
	c.Assert(coll2.Find(bson.M{"a": "2"}).Select(bson.M{"a": 1, "b": 1, "_id": 0}).One(&res), IsNil)
	c.Check(res, DeepEquals, bson.M{"a": "2", "b": "b"})
	c.Assert(session1.CommitTransaction(), IsNil)
	// Now that it is committed, session2 should see it
	c.Assert(coll2.Find(bson.M{"a": "a"}).Select(bson.M{"a": 1, "b": 1, "_id": 0}).One(&res), IsNil)
	c.Check(res, DeepEquals, bson.M{"a": "a", "b": "c"})
	c.Assert(coll2.Find(bson.M{"a": "2"}).Select(bson.M{"a": 1, "b": 1, "_id": 0}).One(&res), IsNil)
	c.Check(res, DeepEquals, bson.M{"a": "2", "b": "c"})
}

func (s *S) TestTransactionUpsertCommitted(c *C) {
	session1, coll1, session2, coll2 := s.setup2Sessions(c)
	defer session1.Close()
	defer session2.Close()
	c.Assert(coll1.Insert(bson.M{"a": "a", "b": "b"}), IsNil)
	c.Assert(session1.StartTransaction(), IsNil)
	// call Abort in case there is a problem, but ignore an error if it was committed,
	// otherwise the server will block in DropCollection because the transaction is active.
	defer session1.AbortTransaction()
	// One Upsert updates, the other Upsert creates
	changeInfo, err := coll1.Upsert(bson.M{"a": "a"}, bson.M{"$set": bson.M{"b": "c"}})
	c.Assert(err, IsNil)
	c.Check(changeInfo.Matched, Equals, 1)
	c.Check(changeInfo.Updated, Equals, 1)
	changeInfo, err = coll1.Upsert(bson.M{"a": "2"}, bson.M{"$set": bson.M{"b": "c"}})
	c.Assert(err, IsNil)
	c.Check(changeInfo.Matched, Equals, 0)
	c.Check(changeInfo.UpsertedId, NotNil)
	// Should be visible in the session that has the transaction
	var res bson.M
	c.Assert(coll1.Find(bson.M{"a": "a"}).Select(bson.M{"a": 1, "b": 1, "_id": 0}).One(&res), IsNil)
	c.Check(res, DeepEquals, bson.M{"a": "a", "b": "c"})
	c.Assert(coll1.Find(bson.M{"a": "2"}).Select(bson.M{"a": 1, "b": 1, "_id": 0}).One(&res), IsNil)
	c.Check(res, DeepEquals, bson.M{"a": "2", "b": "c"})
	// Since the change was made in a transaction, session 2 should not see it
	c.Assert(coll2.Find(bson.M{"a": "a"}).Select(bson.M{"a": 1, "b": 1, "_id": 0}).One(&res), IsNil)
	c.Check(res, DeepEquals, bson.M{"a": "a", "b": "b"})
	c.Assert(coll2.Find(bson.M{"a": "2"}).One(&res), Equals, mgo.ErrNotFound)
	c.Assert(session1.CommitTransaction(), IsNil)
	// Now that it is committed, session2 should see it
	c.Assert(coll2.Find(bson.M{"a": "a"}).Select(bson.M{"a": 1, "b": 1, "_id": 0}).One(&res), IsNil)
	c.Check(res, DeepEquals, bson.M{"a": "a", "b": "c"})
	c.Assert(coll2.Find(bson.M{"a": "2"}).Select(bson.M{"a": 1, "b": 1, "_id": 0}).One(&res), IsNil)
	c.Check(res, DeepEquals, bson.M{"a": "2", "b": "c"})
}

func (s *S) TestTransactionRemoveCommitted(c *C) {
	session1, coll1, session2, coll2 := s.setup2Sessions(c)
	defer session1.Close()
	defer session2.Close()
	c.Assert(coll1.Insert(bson.M{"a": "a", "b": "b"}), IsNil)
	c.Assert(session1.StartTransaction(), IsNil)
	// call Abort in case there is a problem, but ignore an error if it was committed,
	// otherwise the server will block in DropCollection because the transaction is active.
	defer session1.AbortTransaction()
	c.Assert(coll1.Remove(bson.M{"a": "a"}), IsNil)
	// Should be gone in the session that has the transaction
	var res bson.M
	c.Assert(coll1.Find(bson.M{"a": "a"}).One(&res), Equals, mgo.ErrNotFound)
	// Since the change was made in a transaction, session 2 should still see the document
	c.Assert(coll2.Find(bson.M{"a": "a"}).Select(bson.M{"a": 1, "b": 1, "_id": 0}).One(&res), IsNil)
	c.Check(res, DeepEquals, bson.M{"a": "a", "b": "b"})
	c.Assert(session1.CommitTransaction(), IsNil)
	// Now that it is committed, it should be gone
	c.Assert(coll1.Find(bson.M{"a": "a"}).One(&res), Equals, mgo.ErrNotFound)
	c.Assert(coll2.Find(bson.M{"a": "a"}).One(&res), Equals, mgo.ErrNotFound)
}

func (s *S) TestTransactionRemoveAllCommitted(c *C) {
	session1, coll1, session2, coll2 := s.setup2Sessions(c)
	defer session1.Close()
	defer session2.Close()
	c.Assert(coll1.Insert(bson.M{"a": "a", "b": "b"}), IsNil)
	c.Assert(coll1.Insert(bson.M{"a": "2", "b": "b"}), IsNil)
	c.Assert(session1.StartTransaction(), IsNil)
	// call Abort in case there is a problem, but ignore an error if it was committed,
	// otherwise the server will block in DropCollection because the transaction is active.
	defer session1.AbortTransaction()
	changeInfo, err := coll1.RemoveAll(bson.M{"a": bson.M{"$exists": true}})
	c.Assert(err, IsNil)
	c.Check(changeInfo.Matched, Equals, 2)
	c.Check(changeInfo.Removed, Equals, 2)
	// Should be gone in the session that has the transaction
	var res bson.M
	c.Assert(coll1.Find(bson.M{"a": "a"}).One(&res), Equals, mgo.ErrNotFound)
	c.Assert(coll1.Find(bson.M{"a": "2"}).One(&res), Equals, mgo.ErrNotFound)
	// Since the change was made in a transaction, session 2 should still see the document
	c.Assert(coll2.Find(bson.M{"a": "a"}).Select(bson.M{"a": 1, "b": 1, "_id": 0}).One(&res), IsNil)
	c.Check(res, DeepEquals, bson.M{"a": "a", "b": "b"})
	c.Assert(coll2.Find(bson.M{"a": "2"}).Select(bson.M{"a": 1, "b": 1, "_id": 0}).One(&res), IsNil)
	c.Check(res, DeepEquals, bson.M{"a": "2", "b": "b"})
	c.Assert(session1.CommitTransaction(), IsNil)
	// Now that it is committed, it should be gone
	c.Assert(coll1.Find(bson.M{"a": "a"}).One(&res), Equals, mgo.ErrNotFound)
	c.Assert(coll1.Find(bson.M{"a": "2"}).One(&res), Equals, mgo.ErrNotFound)
	c.Assert(coll2.Find(bson.M{"a": "a"}).One(&res), Equals, mgo.ErrNotFound)
	c.Assert(coll2.Find(bson.M{"a": "2"}).One(&res), Equals, mgo.ErrNotFound)
}

func (s *S) TestStartAbortTransactionMultithreaded(c *C) {
	// While calling StartTransaction doesn't actually make sense, it shouldn't corrupt
	// memory to do so. This should trigger go's '-race' detector if we don't
	// have the code structured correctly.
	session := s.setupTxnSession(c)
	defer session.Close()
	// Collections must be created outside of a transaction
	coll1 := session.DB("mydb").C("mycoll")
	err := coll1.Create(&mgo.CollectionInfo{})
	c.Assert(err, IsNil)
	var wg sync.WaitGroup
	startFunc := func() {
		err := session.StartTransaction()
		if err != nil {
			// Don't use Assert as we are being called in a goroutine
			c.Check(err, ErrorMatches, "transaction already started")
		} else {
			c.Check(session.AbortTransaction(), IsNil)
		}
		wg.Done()
	}
	wg.Add(10)
	for i := 0; i < 10; i++ {
		go startFunc()
	}
	wg.Wait()
}

func (s *S) TestStartCommitTransactionMultithreaded(c *C) {
	// While calling StartTransaction doesn't actually make sense, it shouldn't corrupt
	// memory to do so. This should trigger go's '-race' detector if we don't
	// have the code structured correctly.
	session := s.setupTxnSession(c)
	defer session.Close()
	// Collections must be created outside of a transaction
	coll1 := session.DB("mydb").C("mycoll")
	err := coll1.Create(&mgo.CollectionInfo{})
	c.Assert(err, IsNil)
	var wg sync.WaitGroup
	startFunc := func() {
		err := session.StartTransaction()
		if err != nil {
			// Don't use Assert as we are being called in a goroutine
			c.Check(err, ErrorMatches, "transaction already started")
		} else {
			c.Check(session.CommitTransaction(), IsNil)
		}
		wg.Done()
	}
	wg.Add(10)
	for i := 0; i < 10; i++ {
		go startFunc()
	}
	wg.Wait()
}

func (s *S) TestAbortTransactionNotStarted(c *C) {
	session := s.setupTxnSession(c)
	defer session.Close()
	err := session.AbortTransaction()
	c.Assert(err, ErrorMatches, "no transaction in progress")
}

func (s *S) TestCommitTransactionNotStarted(c *C) {
	session := s.setupTxnSession(c)
	defer session.Close()
	err := session.CommitTransaction()
	c.Assert(err, ErrorMatches, "no transaction in progress")
}

func (s *S) TestAbortTransactionNoChanges(c *C) {
	session := s.setupTxnSession(c)
	defer session.Close()
	c.Assert(session.StartTransaction(), IsNil)
	c.Assert(session.AbortTransaction(), IsNil)
}

func (s *S) TestCommitTransactionNoChanges(c *C) {
	session := s.setupTxnSession(c)
	defer session.Close()
	c.Assert(session.StartTransaction(), IsNil)
	c.Assert(session.CommitTransaction(), IsNil)
}

func (s *S) TestAbortTransactionTwice(c *C) {
	session := s.setupTxnSession(c)
	defer session.Close()
	c.Assert(session.StartTransaction(), IsNil)
	c.Assert(session.AbortTransaction(), IsNil)
	err := session.AbortTransaction()
	c.Assert(err, ErrorMatches, "no transaction in progress")
}

func (s *S) TestCommitTransactionTwice(c *C) {
	session := s.setupTxnSession(c)
	defer session.Close()
	c.Assert(session.StartTransaction(), IsNil)
	c.Assert(session.CommitTransaction(), IsNil)
	err := session.CommitTransaction()
	c.Assert(err, ErrorMatches, "no transaction in progress")
}

func (s *S) TestStartCommitAbortStartCommitTransaction(c *C) {
	session1, coll1, session2, coll2 := s.setup2Sessions(c)
	defer session1.Close()
	defer session2.Close()
	c.Assert(session1.StartTransaction(), IsNil)
	c.Assert(session1.CommitTransaction(), IsNil)
	err := session1.AbortTransaction()
	c.Assert(err, ErrorMatches, "no transaction in progress")
	// We should be able to recover
	c.Assert(session1.StartTransaction(), IsNil)
	c.Assert(coll1.Insert(bson.M{"a": "a", "b": "b"}), IsNil)
	c.Assert(session1.CommitTransaction(), IsNil)
	var res bson.M
	// Should be visible in the session that has the transaction
	c.Assert(coll1.Find(bson.M{"a": "a"}).Select(bson.M{"a": 1, "b": 1, "_id": 0}).One(&res), IsNil)
	c.Check(res, DeepEquals, bson.M{"a": "a", "b": "b"})
	c.Assert(coll2.Find(bson.M{"a": "a"}).Select(bson.M{"a": 1, "b": 1, "_id": 0}).One(&res), IsNil)
	c.Check(res, DeepEquals, bson.M{"a": "a", "b": "b"})
}

func (s *S) TestCloseWithOpenTransaction(c *C) {
	session1, coll1, session2, coll2 := s.setup2Sessions(c)
	defer session1.Close()
	defer session2.Close()
	c.Assert(session1.StartTransaction(), IsNil)
	c.Assert(coll1.Insert(bson.M{"a": "a", "b": "b"}), IsNil)
	var res bson.M
	c.Assert(coll1.Find(bson.M{"a": "a"}).Select(bson.M{"a": 1, "b": 1, "_id": 0}).One(&res), IsNil)
	c.Check(res, DeepEquals, bson.M{"a": "a", "b": "b"})
	// Close should abort the current session, which aborts the active transaction
	session1.Close()
	c.Assert(coll2.Find(bson.M{"a": "2"}).One(&res), Equals, mgo.ErrNotFound)
}
